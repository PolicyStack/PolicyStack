# OAuth

The [`oauth`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/oauth) element configures how people log in to a cluster. It writes the cluster's single `OAuth` object, `cluster`. That object lists the identity providers the OpenShift OAuth server offers on the login page (htpasswd, OpenID Connect, LDAP) and how long tokens last. The element can also make a set of users cluster administrators. It installs no operator.

It follows Red Hat's [Configuring identity providers](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/authentication_and_authorization/configuring-identity-providers) and [Configuring the internal OAuth server](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/authentication_and_authorization/configuring-internal-oauth).

## Turn it on

Each entry under `config.identityProviders` is one identity provider, named after the entry. The layer that turns the element on must fill the map. With no enabled provider, the render fails:

```yaml
# values/environments/prod.yaml
stack:
  oauth:
    enabled: true
    config:
      identityProviders:
        local:
          type: HTPasswd
          secret: oauth-htpasswd
      clusterAdmins: [alice]
```

The credentials stay out of Git. Create the hub Secret first, in the policy namespace `pstack`, using the key the provider type expects ([Secrets](#secrets)):

```sh
htpasswd -cbB users.htpasswd alice '<password>'
oc -n pstack create secret generic oauth-htpasswd --from-file=htpasswd=users.htpasswd
```

On each cluster, Policy `config-oauth-<cluster>` then writes:

| Object | What it holds |
|---|---|
| Secret `openshift-config/oauth-<provider>` | A copy of the hub Secret, for each provider with `secret` |
| ConfigMap `openshift-config/oauth-<provider>-ca` | `ca.crt`, for each provider with `ca` |
| `OAuth` `cluster` | Every enabled provider, in name order, plus the [token settings](#token-lifetimes) |
| Group `oauth-cluster-admins` | The users in `clusterAdmins` |
| ClusterRoleBinding `oauth-cluster-admins` | That Group, bound to ClusterRole `cluster-admin` |

`config-oauth-ready` is inform-only. It reports NonCompliant until the `authentication` ClusterOperator is Available and not Degraded. That catches what the API server accepts but the OAuth server can't use: a copied Secret without the expected key, an unreadable CA bundle, an issuer it can't discover. A wrong password, a bad htpasswd line or a wrong client secret shows up only as a failed login, so log in through each provider you change.

!!! danger "The element owns `OAuth` `cluster`"
    Every object uses `mustonlyhave`. On the first evaluation, any identity provider the element doesn't declare is removed from `OAuth` `cluster`. That includes providers added by hand and `spec.templates` (custom login pages). Before turning the element on, declare every provider already in `oc get oauth cluster -o yaml`. For the same reason the render fails when no provider is enabled: an empty list would remove every way in except `kubeadmin`.

!!! warning "Hosted control planes"
    Every object is skipped on a hosted control plane (HCP) guest, a cluster whose `Infrastructure` reports `controlPlaneTopology: External`. HyperShift writes the guest's `OAuth` from `spec.configuration.oauth` on the HostedCluster and overwrites edits made in the guest. Set identity providers there instead. The skipped templates show Compliant.

## Identity providers

Each entry needs `type`. `mappingMethod` sets how a login maps to a `User`. It can be `claim` (the default), `lookup` or `add` ([Identity provider parameters](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/authentication_and_authorization/understanding-identity-provider#identity-provider-parameters_understanding-identity-provider)). With `claim`, a second provider returning a user name that already exists is refused. `add` maps it to the same `User` instead. Use it only when every provider names the same people the same way. `generate` isn't offered.

!!! warning "`add` shares cluster-admin across providers"
    `clusterAdmins` grants by `User` name. Under `add`, any provider that returns `alice` logs in as the `User` `alice`, with her `cluster-admin`. The same applies to an OpenID `preferredUsername` claim that users can set themselves. Keep `claim` unless the providers share one directory.

=== "HTPasswd"

    | Key | Required | Meaning |
    |---|---|---|
    | `secret` | yes | Hub Secret holding the htpasswd file under key `htpasswd` |

=== "OpenID"

    ```yaml
    sso:
      type: OpenID
      secret: sso-client
      clientID: openshift
      issuer: https://sso.example.com/realms/openshift
      claims:
        preferredUsername: [preferred_username]
        name: [name]
        email: [email]
        groups: [groups]
      ca: |
        -----BEGIN CERTIFICATE-----
        ...
        -----END CERTIFICATE-----
    ```

    | Key | Required | Meaning |
    |---|---|---|
    | `secret` | yes | Hub Secret holding the client secret under key `clientSecret` |
    | `clientID` | yes | The client registered with the provider |
    | `issuer` | yes | Issuer URL, `https://`, without query, fragment or trailing `/` |
    | `claims` | | `preferredUsername`, `name`, `email`, `groups`: lists of claims, first non-empty wins. Unset, the user name is the `sub` claim |
    | `extraScopes`, `extraAuthorizeParameters` | | Extra scopes, one per list entry, and authorize request parameters. Quote parameter values: `max_age: "300"` |
    | `ca` | | PEM bundle that verifies the issuer. Unset, the system roots are used |

    Register the redirect URI `https://<oauth host>/oauth2callback/<provider name>` with the provider, where `<oauth host>` is `oc -n openshift-authentication get route oauth-openshift -o jsonpath='{.spec.host}'`. With `claims.groups`, the OAuth server creates a Group for each value of the claim and keeps its members in step at every login (see [Cluster admins](#cluster-admins)).

=== "LDAP"

    ```yaml
    corp:
      type: LDAP
      url: ldaps://ldap.example.com/ou=users,dc=example,dc=com?uid
      bindDN: cn=openshift,ou=services,dc=example,dc=com
      secret: ldap-bind
      attributes: {id: [dn], preferredUsername: [uid], name: [cn], email: [mail]}
    ```

    | Key | Required | Meaning |
    |---|---|---|
    | `url` | yes | RFC 2255 search URL, `ldap://` or `ldaps://` |
    | `attributes` | `id` | `id`, `preferredUsername`, `name`, `email`: lists of attributes, first non-empty wins |
    | `bindDN` and `secret` | | Bind for the search; set both or neither. The hub Secret holds the password under key `bindPassword` |
    | `insecure` | | `true` connects to an `ldap://` URL without TLS. `false` (default) upgrades `ldap://` with StartTLS. Fails with `ldaps://`, which always uses TLS, or with `ca` |
    | `ca` | | PEM bundle that verifies the server. Unset, the system roots are used |

A key that doesn't belong to the type, a missing required key or a malformed value fails the render with the path of the entry. `enabled: false` removes a provider from `OAuth`. Its copied Secret stays on the cluster, unused.

!!! warning "Keep provider names"
    A provider's name prefixes every identity it creates (`local:alice`). Renaming a provider creates new identities. Under `claim`, the existing users then can't log in ("already mapped"), and an OpenID provider needs its redirect URI registered again. Names are 1-50 lowercase letters, digits and hyphens, because they also name the copied Secret and ConfigMap.

## Secrets

The template copies each provider's hub Secret from `pstack` to `openshift-config/oauth-<provider>` with `copySecretData`, the same mechanism as [Logging](logging.md#secrets). The OAuth server reads one key from it:

| Type | Hub Secret key |
|---|---|
| HTPasswd | `htpasswd` |
| OpenID | `clientSecret` |
| LDAP | `bindPassword` |

```sh
oc -n pstack create secret generic sso-client --from-literal=clientSecret=<secret>
oc -n pstack create secret generic ldap-bind --from-literal=bindPassword=<password>
```

A change to the hub Secret reaches every cluster without a commit, so adding an htpasswd user is a hub-side `oc set data`. `copySecretData` copies every key, so keep only the one key in the hub Secret.

Secrets, CA bundles and `OAuth` are in one template on purpose. If a hub Secret is missing, the hub template fails for the whole template. `OAuth` is then never written naming a Secret the cluster doesn't have. The Policy reports the hub template error, and the cluster keeps what was last applied. A copied Secret without the expected key isn't caught here: `config-oauth-ready` reports it. A wrong password inside it shows only as a failed login.

## Cluster admins

`clusterAdmins` lists user names: the `User` names OpenShift shows, such as `alice` with `claim` mapping. They go into Group `oauth-cluster-admins`, which ClusterRoleBinding `oauth-cluster-admins` binds to `cluster-admin`. The element owns the Group, so a user added to it by hand is removed. Removing a name from `clusterAdmins` revokes that user's `cluster-admin` at the next evaluation.

- Empty `clusterAdmins` writes neither object. Emptying it later leaves the Group and binding as last written. To revoke the last admin, delete Group `oauth-cluster-admins` by hand.
- Don't use `oauth-cluster-admins` as a group name in your OpenID `groups` claim or LDAP group sync. The OAuth server and the element would keep rewriting each other's member list.
- To grant `cluster-admin` to a group your identity provider already manages, bind it yourself or through another element. The element writes only its own Group.
- [ACS](acs.md) maps the OpenShift group `cluster-admins` to its Admin role by default. To give these users ACS Admin as well, add `oauth-cluster-admins: [Admin]` under `stack.acs.config.auth.groups`.

## Token lifetimes

| Key | Default | Meaning |
|---|---|---|
| `accessTokenMaxAgeSeconds` | `0`, the server default of 86400 (24 hours) | How long an access token lives |
| `accessTokenInactivityTimeout` | empty, no timeout | A token unused this long stops working. A duration such as `15m` or `1h30m`, at least `300s` |

Both are written to `spec.tokenConfig` only when set. A change applies to tokens issued afterwards: existing tokens keep their lifetime and timeout ([Configuring token inactivity timeout](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/authentication_and_authorization/configuring-internal-oauth#oauth-token-inactivity-timeout_configuring-internal-oauth)).

## Removing kubeadmin

This element creates the two things [API Hardening's `kubeadmin` Policy](api-hardening.md#removing-kubeadmin) waits for: an identity provider and, once `clusterAdmins` is set, a cluster-admin binding to a non-system Group. The two elements don't depend on each other. API Hardening reads the cluster state, so it works the same with providers configured by hand.

By default that check doesn't prove anyone can log in, so log in as one of `clusterAdmins` before turning the `kubeadmin` Policy on. `kubeadminRequireLogin: true` in API Hardening also makes removal wait for two things. First, a user bound to `cluster-admin` (through this Group or any other binding) has logged in through a provider `OAuth` lists. Second, the authentication operator has settled. That proves a login once worked under that provider name, not that it works with today's credentials. On a cluster adopted with existing users, or after changing a provider's hub Secret, log in again before turning the Policy on.

```yaml
stack:
  apiHardening:
    enabled: true
    config:
      kubeadminRequireLogin: true
    policies:
      kubeadmin:
        enabled: true
```

## Turning it off

The element sets no pruning. Disabling it leaves `OAuth`, the copied Secrets, the Group and the `cluster-admin` binding as last written. Emptying `clusterAdmins` first doesn't revoke anyone either ([Cluster admins](#cluster-admins)). Delete Group and ClusterRoleBinding `oauth-cluster-admins` by hand. Removing a provider or an htpasswd user stops new logins but doesn't end existing sessions. Tokens already issued last until they expire, 24 hours by default. To end them sooner, delete the user's `OAuthAccessToken` objects:

```sh
oc delete oauthaccesstokens --field-selector userName=alice
```
