// Package imageset builds the oc-mirror v2 ImageSetConfiguration a set of fleet files needs:
// OpenShift releases from their version and channel, and operator packages and channels from the
// OperatorPolicies their elements render, read before global.catalogSources renames the source.
package imageset

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/reporter"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/run"
)

// catalogs maps the CatalogSources OpenShift ships to their index images, tagged v<minor>.
var catalogs = map[string]string{
	"redhat-operators":    "registry.redhat.io/redhat/redhat-operator-index",
	"certified-operators": "registry.redhat.io/redhat/certified-operator-index",
	"community-operators": "registry.redhat.io/redhat/community-operator-index",
	"redhat-marketplace":  "registry.redhat.io/redhat/redhat-marketplace-index",
}

// minorSuffix reads the OpenShift minor from a channel name such as stable-4.20.
var minorSuffix = regexp.MustCompile(`-(\d+\.\d+)$`)

// csvVersion reads the bundle version from a CSV name such as advanced-cluster-management.v2.17.1
// or nfd.4.20.0-202511040403.
var csvVersion = regexp.MustCompile(`\.v?(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.+-]*)?)$`)

// Package is a redhat-operators package no element subscribes to, such as one an operator installs
// itself. An empty Channel mirrors the head of every channel.
type Package struct{ Name, Channel string }

// ParsePackage parses name[:channel].
func ParsePackage(s string) (Package, error) {
	name, channel, _ := strings.Cut(s, ":")
	if name == "" || strings.Contains(s, ":") && channel == "" {
		return Package{}, fmt.Errorf("package %q: want name[:channel]", s)
	}
	return Package{name, channel}, nil
}

// Cluster is one fleet file and what its Applications render.
type Cluster struct {
	Name, Version, Channel string
	Rendered               []byte
}

// Generate renders the fleet files in opts.FixturesDir named in names, or every one when names is
// empty, as run.Run does, and returns Build's output for them.
func Generate(ctx context.Context, opts run.Options, names []string, extra []Package) ([]byte, error) {
	fleet, err := fixtures.LoadDir(opts.FixturesDir)
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		var errs []error
		for _, n := range names {
			if !slices.ContainsFunc(fleet, func(c *fixtures.Cluster) bool { return c.Name == n }) {
				errs = append(errs, fmt.Errorf("%s: no fleet file in %s", n, opts.FixturesDir))
			}
		}
		if err := errors.Join(errs...); err != nil {
			return nil, err
		}
		fleet = slices.DeleteFunc(fleet, func(c *fixtures.Cluster) bool { return !slices.Contains(names, c.Name) })
	}

	dir, err := os.MkdirTemp("", "imageset-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// Ranked above every layer, so each OperatorPolicy keeps the source its element names.
	unmap := filepath.Join(dir, "catalog-sources.yaml")
	if err := os.WriteFile(unmap, []byte("global:\n  catalogSources: null\n"), 0o600); err != nil {
		return nil, err
	}
	opts.ExtraValues = append(slices.Clip(opts.ExtraValues), unmap)
	opts.RenderDir = filepath.Join(dir, "render")
	opts.Only = []string{"RENDER000", "POLICY050"}
	targets := slices.Compact(slices.Sorted(slices.Values(names)))
	if len(targets) == 0 {
		targets = []string{""}
	}
	for _, n := range targets {
		opts.Cluster = n
		res, err := run.Run(ctx, opts)
		if err != nil {
			return nil, err
		}
		if res.Errors > 0 {
			var b bytes.Buffer
			_ = (reporter.Pretty{NoColor: true}).Write(&b, res.Findings)
			return nil, fmt.Errorf("render failed:\n%s", b.String())
		}
	}

	clusters := make([]Cluster, 0, len(fleet))
	for _, fc := range fleet {
		rel, err := filepath.Rel(filepath.Clean(opts.FixturesDir), fc.SourceFile)
		if err != nil {
			return nil, err
		}
		files, err := filepath.Glob(filepath.Join(opts.RenderDir, strings.TrimSuffix(rel, ".yaml"), "*.yaml"))
		if err != nil {
			return nil, err
		}
		c := Cluster{Name: fc.Name, Version: fc.Version, Channel: fc.Channel}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			c.Rendered = append(append(c.Rendered, "\n---\n"...), data...)
		}
		clusters = append(clusters, c)
	}
	return Build(clusters, extra)
}

// imageSetConfiguration is the part of oc-mirror's mirror.openshift.io/v2alpha1 schema Build writes.
type imageSetConfiguration struct {
	Kind       string `yaml:"kind"`
	APIVersion string `yaml:"apiVersion"`
	Mirror     struct {
		Platform  *platform  `yaml:"platform,omitempty"`
		Operators []operator `yaml:"operators,omitempty"`
	} `yaml:"mirror"`
}

type platform struct {
	Channels []channel `yaml:"channels"`
	Graph    bool      `yaml:"graph"`
}

type channel struct {
	Name       string `yaml:"name"`
	MinVersion string `yaml:"minVersion,omitempty"`
	MaxVersion string `yaml:"maxVersion,omitempty"`
}

type operator struct {
	Catalog  string        `yaml:"catalog"`
	Packages []pkgChannels `yaml:"packages"`
}

type pkgChannels struct {
	Name string `yaml:"name"`
	// DefaultChannel must name a listed channel when the catalog's default is not listed, or
	// oc-mirror rejects the catalog. Every subscription it serves names its channel.
	DefaultChannel string    `yaml:"defaultChannel,omitempty"`
	Channels       []channel `yaml:"channels,omitempty"`
}

// versions are the releases a channel must hold. Without max, oc-mirror mirrors from min to the
// channel head, and without either only the head.
type versions struct {
	min, max *semver.Version
	unpinned bool
}

func (v *versions) add(ver *semver.Version) {
	if v.min == nil || ver.LessThan(v.min) {
		v.min = ver
	}
	if v.max == nil || ver.GreaterThan(v.max) {
		v.max = ver
	}
}

func (v *versions) channel(name string) channel {
	c := channel{Name: name}
	if v.min != nil {
		c.MinVersion = v.min.Original()
	}
	if v.max != nil && !v.unpinned {
		c.MaxVersion = v.max.Original()
	}
	return c
}

// operatorPolicy is the part of a rendered Policy's OperatorPolicy templates Build reads.
type operatorPolicy struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		ComplianceType string   `json:"complianceType"`
		Versions       []string `json:"versions"`
		Subscription   struct {
			Name        string `json:"name"`
			Channel     string `json:"channel"`
			Source      string `json:"source"`
			StartingCSV string `json:"startingCSV"`
		} `json:"subscription"`
	} `json:"spec"`
}

// Build returns the ImageSetConfiguration clusters need. Each cluster's OpenShift minor, from
// version or else channel, tags the catalogs its OperatorPolicies subscribe from, and extra joins
// redhat-operators at every minor. A package from any other catalog, whose index image is unknown,
// is left out and named in a comment. A cluster with version adds it to its channel, stable-<minor> by
// default, the platform channels spanning the lowest to the highest version. Pinned versions and
// startingCSV bound an operator channel the same way, with no upper bound while any subscription to
// it pins none. A package subscribed without a channel gets the head of every channel, so it must
// pin nothing, and no other subscription to it may name a channel.
func Build(clusters []Cluster, extra []Package) ([]byte, error) {
	releases := map[string]*versions{}
	// catalog image -> package -> channel ("" for the default channel)
	ops := map[string]map[string]map[string]*versions{}
	use := func(catalog, pkg, ch string) *versions {
		if ops[catalog] == nil {
			ops[catalog] = map[string]map[string]*versions{}
		}
		if ops[catalog][pkg] == nil {
			ops[catalog][pkg] = map[string]*versions{}
		}
		if ops[catalog][pkg][ch] == nil {
			ops[catalog][pkg][ch] = &versions{}
		}
		return ops[catalog][pkg][ch]
	}

	minors := map[string]bool{}
	// source -> packages, for sources that are not a catalog OpenShift ships
	unknown := map[string]map[string]bool{}
	var errs []error
	for _, c := range clusters {
		minor, err := clusterMinor(c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		minors[minor] = true
		if c.Version != "" {
			v, _ := semver.StrictNewVersion(c.Version) // clusterMinor parsed it
			ch := cmp.Or(c.Channel, "stable-"+minor)
			if releases[ch] == nil {
				releases[ch] = &versions{}
			}
			releases[ch].add(v)
		}
		policies, err := operatorPolicies(c.Rendered)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
			continue
		}
		for _, op := range policies {
			s := op.Spec.Subscription
			source := cmp.Or(s.Source, "redhat-operators")
			if templated(s.Name, s.Channel, source, s.StartingCSV) || templated(op.Spec.Versions...) {
				errs = append(errs, fmt.Errorf("%s: OperatorPolicy %s: a hub or managed cluster template cannot be resolved here", c.Name, op.Metadata.Name))
				continue
			}
			image, ok := catalogs[source]
			if !ok {
				if unknown[source] == nil {
					unknown[source] = map[string]bool{}
				}
				unknown[source][s.Name] = true
				continue
			}
			v := use(image+":v"+minor, s.Name, s.Channel)
			if len(op.Spec.Versions) == 0 {
				v.unpinned = true
			}
			for _, entry := range append(slices.Clip(op.Spec.Versions), s.StartingCSV) {
				for csv := range strings.SplitSeq(entry, ",") {
					if csv = strings.TrimSpace(csv); csv == "" {
						continue
					}
					ver, err := bundleVersion(csv)
					if err != nil {
						errs = append(errs, fmt.Errorf("%s: OperatorPolicy %s: %w", c.Name, op.Metadata.Name, err))
						continue
					}
					v.add(ver)
				}
			}
		}
	}
	for minor := range minors {
		for _, p := range extra {
			use(catalogs["redhat-operators"]+":v"+minor, p.Name, p.Channel).unpinned = true
		}
	}
	for _, catalog := range slices.Sorted(maps.Keys(ops)) {
		for _, name := range slices.Sorted(maps.Keys(ops[catalog])) {
			switch v, ok := ops[catalog][name][""]; {
			case ok && len(ops[catalog][name]) > 1:
				errs = append(errs, fmt.Errorf("%s in %s: subscribed both with and without a channel: name the channel", name, catalog))
			case ok && v.min != nil:
				errs = append(errs, fmt.Errorf("%s in %s: pinned without a channel: name the channel", name, catalog))
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	out := imageSetConfiguration{Kind: "ImageSetConfiguration", APIVersion: "mirror.openshift.io/v2alpha1"}
	if len(releases) > 0 {
		out.Mirror.Platform = &platform{Channels: channels(releases), Graph: true}
	}
	for _, catalog := range slices.Sorted(maps.Keys(ops)) {
		o := operator{Catalog: catalog}
		for _, name := range slices.Sorted(maps.Keys(ops[catalog])) {
			chs := ops[catalog][name]
			// The only subscription naming no channel: an empty list mirrors every channel's head.
			delete(chs, "")
			p := pkgChannels{Name: name, Channels: channels(chs)}
			if len(p.Channels) > 0 {
				p.DefaultChannel = p.Channels[0].Name
			}
			o.Packages = append(o.Packages, p)
		}
		out.Mirror.Operators = append(out.Mirror.Operators, o)
	}
	var b bytes.Buffer
	for _, source := range slices.Sorted(maps.Keys(unknown)) {
		fmt.Fprintf(&b, "# Not mirrored: %s from %s, not a catalog OpenShift ships.\n",
			strings.Join(slices.Sorted(maps.Keys(unknown[source])), ", "), source)
	}
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return b.Bytes(), enc.Close()
}

func channels(m map[string]*versions) []channel {
	var out []channel
	for _, name := range slices.Sorted(maps.Keys(m)) {
		out = append(out, m[name].channel(name))
	}
	return out
}

// clusterMinor returns the OpenShift minor of c: from version, else from channel.
func clusterMinor(c Cluster) (string, error) {
	if c.Version != "" {
		v, err := semver.StrictNewVersion(c.Version)
		if err != nil {
			return "", fmt.Errorf("%s: version %q is not an x.y.z release", c.Name, c.Version)
		}
		return fmt.Sprintf("%d.%d", v.Major(), v.Minor()), nil
	}
	if m := minorSuffix.FindStringSubmatch(c.Channel); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("%s: set version, or a channel such as stable-4.20, in its fleet file: the OpenShift minor tags its catalogs", c.Name)
}

// bundleVersion returns the version in a CSV name.
func bundleVersion(csv string) (*semver.Version, error) {
	m := csvVersion.FindStringSubmatch(csv)
	if m == nil {
		return nil, fmt.Errorf("%q: no x.y.z version at the end of the CSV name", csv)
	}
	v, err := semver.StrictNewVersion(m[1])
	if err != nil {
		return nil, fmt.Errorf("%q: %w", csv, err)
	}
	return v, nil
}

// operatorPolicies returns the OperatorPolicies in the rendered Policies, but those that remove an
// operator.
func operatorPolicies(rendered []byte) ([]operatorPolicy, error) {
	var out []operatorPolicy
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		var p struct {
			Kind string `json:"kind"`
			Spec struct {
				PolicyTemplates []struct {
					ObjectDefinition operatorPolicy `json:"objectDefinition"`
				} `json:"policy-templates"`
			} `json:"spec"`
		}
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			return out, nil
		} else if err != nil {
			return nil, err
		}
		if p.Kind != "Policy" {
			continue
		}
		for _, t := range p.Spec.PolicyTemplates {
			if op := t.ObjectDefinition; op.Kind == "OperatorPolicy" && !strings.EqualFold(op.Spec.ComplianceType, "mustnothave") {
				out = append(out, op)
			}
		}
	}
}

func templated(s ...string) bool {
	return slices.ContainsFunc(s, func(v string) bool { return strings.Contains(v, "{{") })
}
