#!/usr/bin/env python3
"""
PolicyStack Documentation Generator

Generates markdown documentation for PolicyStack elements by parsing values.yaml files
and extracting configuration details with support for inline comment annotations at any level.

Usage:
    python doc-generator.py [--output-dir docs] [--stack-dir stack]
    python doc-generator.py --check  # Check if docs are up to date
"""

import os
import re
import yaml
import argparse
import tempfile
import difflib
import sys
from pathlib import Path
from typing import Dict, List, Any, Optional, Tuple, Union
from dataclasses import dataclass, field
from datetime import datetime
from collections import defaultdict


@dataclass
class DocumentedValue:
    """Represents a YAML value with its associated documentation"""
    value: Any
    description: Optional[str] = None
    field_descriptions: Dict[str, str] = field(default_factory=dict)


class YAMLLoader:
    """YAML loader that preserves comments at any nesting level"""
    
    def __init__(self, file_path: Path):
        self.file_path = file_path
        self.comments = {}  # Flat dictionary of path -> description
        self.nested_comments = defaultdict(dict)  # Nested structure matching YAML
        
        with open(file_path, 'r') as f:
            self.raw_content = f.read()
        
        self._parse_with_comments()
    
    def load(self) -> Tuple[Dict, Dict]:
        """Load YAML and return both data and comments"""
        with open(self.file_path, 'r') as f:
            data = yaml.safe_load(f) or {}
        return data, self.nested_comments
    
    def _parse_with_comments(self):
        """Parse YAML with comment preservation at all levels"""
        lines = self.raw_content.split('\n')
        current_path = []
        indent_stack = [0]
        pending_description = None
        array_indices = defaultdict(int)
        
        for i, line in enumerate(lines):
            # Check for description comment
            desc_match = re.match(r'^(\s*)#\s*@(desc|description):\s*(.+)$', line)
            if desc_match:
                pending_description = desc_match.group(3).strip()
                continue
            
            # Skip regular comments and empty lines
            if line.strip().startswith('#') or not line.strip():
                continue
            
            # Get indentation level
            indent = len(line) - len(line.lstrip())
            
            # Parse YAML line
            # Handle array items
            array_match = re.match(r'^(\s*)- (.+)$', line)
            if array_match:
                indent = len(array_match.group(1))
                content = array_match.group(2)
                
                # Adjust path based on indentation
                while indent_stack and indent <= indent_stack[-1]:
                    indent_stack.pop()
                    if current_path:
                        popped = current_path.pop()
                        # Reset array index if we've left this array
                        if isinstance(popped, int):
                            parent_key = current_path[-1] if current_path else ''
                            array_indices[parent_key] = 0
                
                # Add array index to path
                parent_key = '.'.join(str(p) for p in current_path)
                current_index = array_indices[parent_key]
                current_path.append(current_index)
                array_indices[parent_key] = current_index + 1
                
                # Check if this array item has a name field
                name_match = re.match(r'name:\s*(.+)$', content)
                if name_match:
                    name_value = name_match.group(1).strip().strip('"\'')
                    if pending_description:
                        # Store description for this array item by name
                        path_key = '.'.join(str(p) for p in current_path[:-1])
                        self._set_nested_value(self.nested_comments, 
                                             path_key + f'.{name_value}', 
                                             pending_description)
                        pending_description = None
                
                # Handle inline key-value in array item
                kv_match = re.match(r'(\w+):\s*(.+)$', content)
                if kv_match and pending_description:
                    key = kv_match.group(1)
                    path_key = '.'.join(str(p) for p in current_path) + f'.{key}'
                    self._set_nested_value(self.nested_comments, path_key, pending_description)
                    pending_description = None
                
                indent_stack.append(indent)
            else:
                # Handle regular key-value pairs
                kv_match = re.match(r'^(\s*)([^:]+):\s*(.*)$', line)
                if kv_match:
                    indent = len(kv_match.group(1))
                    key = kv_match.group(2).strip()
                    value = kv_match.group(3).strip()
                    
                    # Adjust path based on indentation
                    while indent_stack and indent < indent_stack[-1]:
                        indent_stack.pop()
                        if current_path:
                            popped = current_path.pop()
                            # Reset array index if we've left an array
                            if isinstance(popped, int):
                                parent_key = '.'.join(str(p) for p in current_path) if current_path else ''
                                array_indices[parent_key] = 0
                    
                    # Update or append to current path
                    if indent_stack and indent > indent_stack[-1]:
                        current_path.append(key)
                        indent_stack.append(indent)
                    else:
                        if current_path and indent == indent_stack[-1]:
                            current_path[-1] = key
                        else:
                            current_path.append(key)
                            indent_stack.append(indent)
                    
                    # Store description if we have one pending
                    if pending_description:
                        path_key = '.'.join(str(p) for p in current_path)
                        self._set_nested_value(self.nested_comments, path_key, pending_description)
                        pending_description = None
    
    def _set_nested_value(self, nested_dict: Dict, path: str, value: Any):
        """Set a value in a nested dictionary using dot notation path"""
        keys = path.split('.')
        current = nested_dict
        
        for key in keys[:-1]:
            # If current is a string (a description), we need to convert it to a dict
            # with a special __desc__ key to store the description
            if isinstance(current, str):
                # This shouldn't happen with our structure, but handle it gracefully
                temp_desc = current
                current = {'__desc__': temp_desc}
                # Update the parent to point to this new dict
                # This is tricky, we need to navigate again
                temp = nested_dict
                for k in keys[:keys.index(key)]:
                    temp = temp[k]
                temp[key] = current
            elif key not in current:
                current[key] = {}
            elif isinstance(current[key], str):
                # Convert string to dict with __desc__ key
                current[key] = {'__desc__': current[key]}
            
            current = current[key]
        
        # Set the final value
        final_key = keys[-1]
        if isinstance(current, str):
            # This shouldn't happen, but handle it
            return
        
        if final_key in current and isinstance(current[final_key], dict):
            # If there's already a dict here, store the description as __desc__
            current[final_key]['__desc__'] = value
        else:
            # Otherwise, just set the value
            current[final_key] = value
    
    def get_description(self, path: Union[str, List]) -> Optional[str]:
        """Get description for a specific path"""
        if isinstance(path, list):
            path = '.'.join(str(p) for p in path)
        
        keys = path.split('.')
        current = self.nested_comments
        
        for key in keys:
            if isinstance(current, dict) and key in current:
                current = current[key]
            else:
                return None
        
        # Handle both direct string descriptions and dict with __desc__ key
        if isinstance(current, str):
            return current
        elif isinstance(current, dict) and '__desc__' in current:
            return current['__desc__']
        else:
            return None


class DocumentationGenerator:
    """Generates markdown documentation for PolicyStack elements"""
    
    def __init__(self, stack_dir: str = "stack", output_dir: str = "docs", check_mode: bool = False):
        self.stack_dir = Path(stack_dir)
        self.output_dir = Path(output_dir)
        self.check_mode = check_mode
        
        if not check_mode:
            self.output_dir.mkdir(exist_ok=True)
        
    def normalize_timestamp(self, content: str) -> str:
        """Replace timestamp with a placeholder for comparison"""
        # Replace any timestamp in the format YYYY-MM-DD HH:MM:SS with a placeholder
        return re.sub(
            r'\*Generated: \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\*',
            '*Generated: TIMESTAMP_PLACEHOLDER*',
            content
        )
    
    def compare_content(self, existing_content: str, new_content: str) -> bool:
        """Compare content ignoring timestamp differences"""
        normalized_existing = self.normalize_timestamp(existing_content)
        normalized_new = self.normalize_timestamp(new_content)
        return normalized_existing == normalized_new
    
    def to_camel_case(self, name: str) -> str:
        """Convert kebab-case to camelCase"""
        parts = name.split('-')
        return parts[0].lower() + ''.join(p.capitalize() for p in parts[1:])
    
    def load_yaml_with_comments(self, file_path: Path) -> Tuple[Dict, Dict]:
        """Load YAML file and extract comments at all levels"""
        try:
            loader = YAMLLoader(file_path)
            data, comments = loader.load()
            return data, comments
        except Exception as e:
            print(f"Warning: Could not parse comments from {file_path}: {e}")
            print(f"Falling back to basic YAML loading without comments")
            # Fall back to basic YAML loading without comments
            with open(file_path, 'r') as f:
                data = yaml.safe_load(f) or {}
            return data, {}
    
    def get_field_description(self, comments: Dict, *path_parts) -> Optional[str]:
        """Get description for a field at any nesting level"""
        current = comments
        
        for part in path_parts:
            part_str = str(part)
            if isinstance(current, dict):
                # Try exact match first
                if part_str in current:
                    current = current[part_str]
                # Try numeric index for arrays
                elif part_str.isdigit() and part_str in current:
                    current = current[part_str]
                # Try by name if it's in the current dict
                elif isinstance(part, str) and part in current:
                    current = current[part]
                else:
                    return None
            elif isinstance(current, str):
                # We've reached a description but still have path parts left
                return None
            else:
                return None
        
        # Extract description from the final value
        if isinstance(current, str):
            return current
        elif isinstance(current, dict) and '__desc__' in current:
            return current['__desc__']
        else:
            return None
    
    def generate_element_docs(self, element_path: Path) -> Optional[str]:
        """Generate documentation for a single element"""
        values_file = element_path / "values.yaml"
        chart_file = element_path / "Chart.yaml"
        
        if not values_file.exists():
            return None
            
        # Load chart metadata
        chart_name = element_path.name
        chart_description = "No description available"
        
        if chart_file.exists():
            with open(chart_file, 'r') as f:
                chart_data = yaml.safe_load(f)
                chart_name = chart_data.get('name', chart_name)
                chart_description = chart_data.get('description', chart_description)
        
        # Load values with comments
        values, comments = self.load_yaml_with_comments(values_file)
        
        # Get the component name (camelCase version of chart name)
        component_name = self.to_camel_case(chart_name)
        
        # Get the component configuration
        component = values.get('stack', {}).get(component_name, {})
        
        if not component:
            return None
            
        # Generate markdown
        md = []
        md.append(f"# {chart_name} - Policy Library Documentation")
        md.append("")
        md.append(f"> {chart_description}")
        md.append("")
        md.append(f"*Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}*")
        md.append("")
        
        # Component Configuration
        md.append("## Component Configuration")
        md.append("")
        md.append("| Parameter | Value | Description |")
        md.append("| --------- | ----- | ----------- |")
        
        # Get component-level description
        comp_desc = self.get_field_description(comments, 'stack', component_name) or "Main component configuration"
        md.append(f"| Component | `{component_name}` | {comp_desc} |")
        
        enabled_desc = self.get_field_description(comments, 'stack', component_name, 'enabled') or "Whether this component is enabled"
        md.append(f"| Enabled | `{component.get('enabled', False)}` | {enabled_desc} |")
        
        if component.get('disablePlacements'):
            desc = self.get_field_description(comments, 'stack', component_name, 'disablePlacements') or "Whether automatic placement generation is disabled"
            md.append(f"| Disable Placements | `{component.get('disablePlacements')}` | {desc} |")
        
        if component.get('usePolicySetsPlacements'):
            desc = self.get_field_description(comments, 'stack', component_name, 'usePolicySetsPlacements') or "Whether placements bind to PolicySets instead of Policies"
            md.append(f"| Use PolicySet Placements | `{component.get('usePolicySetsPlacements')}` | {desc} |")
        
        md.append("")
        
        # Default Values
        if component.get('default'):
            md.append("## Default Policy Metadata")
            md.append("")
            desc = self.get_field_description(comments, 'stack', component_name, 'default') or "These defaults are applied to all policies unless overridden."
            md.append(desc)
            md.append("")
            md.append("| Type | Values | Description |")
            md.append("| ---- | ------ | ----------- |")
            
            defaults = component['default']
            if defaults.get('categories'):
                desc = self.get_field_description(comments, 'stack', component_name, 'default', 'categories') or "Default category classifications"
                md.append(f"| Categories | {', '.join(defaults['categories'])} | {desc} |")
            if defaults.get('controls'):
                desc = self.get_field_description(comments, 'stack', component_name, 'default', 'controls') or "Default control mappings"
                md.append(f"| Controls | {', '.join(defaults['controls'])} | {desc} |")
            if defaults.get('standards'):
                desc = self.get_field_description(comments, 'stack', component_name, 'default', 'standards') or "Default compliance standards"
                md.append(f"| Standards | {', '.join(defaults['standards'])} | {desc} |")
            
            md.append("")
        
        # Toggles override an entry's `enabled` by name. They are a map, so a cluster values file
        # can flip one sub-feature without restating the whole list.
        toggles = component.get('toggles') or {}
        if toggles:
            md.append("## Sub-Feature Toggles")
            md.append("")
            md.append("Override an entry's `enabled` by name. Being a map, these merge cleanly "
                      "through the values cascade, so a per-cluster override stays one line.")
            md.append("")
            md.append("| Toggle | Default | Description |")
            md.append("| ------ | ------- | ----------- |")
            for tname, tval in toggles.items():
                desc = self.get_field_description(comments, 'stack', component_name, 'toggles', tname) or ""
                md.append(f"| `{tname}` | `{tval}` | {desc} |")
            md.append("")

        # config: holds the tunables the values cascade is expected to override per cluster.
        config = component.get('config') or {}
        if config:
            md.append("## Configuration")
            md.append("")
            md.append("Values intended to be overridden per environment, datacenter, or cluster.")
            md.append("")
            md.append("| Key | Default | Description |")
            md.append("| --- | ------- | ----------- |")
            for ckey, cval in config.items():
                desc = self.get_field_description(comments, 'stack', component_name, 'config', ckey) or ""
                shown = cval if not isinstance(cval, (dict, list)) else f"({type(cval).__name__})"
                md.append(f"| `{ckey}` | `{shown}` | {desc} |")
            md.append("")

        # Process Policies
        policies = component.get('policies', [])
        if policies:
            md.append("## Policies")
            md.append("")
            
            for idx, policy in enumerate(policies):
                if not policy.get('enabled', False):
                    continue
                    
                self._generate_policy_section(md, policy, component, comments, 
                                            ['stack', component_name, 'policies', idx])
        
        # Process PolicySets
        policy_sets = component.get('policySets', [])
        if policy_sets:
            md.append("## PolicySets")
            md.append("")
            
            for idx, policy_set in enumerate(policy_sets):
                if not policy_set.get('enabled', False):
                    continue
                    
                md.append(f"### 📦 PolicySet: {policy_set['name']}")
                
                # Try to get description by name or index
                desc = (policy_set.get('description') or 
                       self.get_field_description(comments, 'stack', component_name, 'policySets', policy_set['name']) or
                       self.get_field_description(comments, 'stack', component_name, 'policySets', idx))
                if desc:
                    md.append(f"> {desc}")
                
                md.append("")
                md.append("| Parameter | Value | Description |")
                md.append("| --------- | ----- | ----------- |")
                md.append(f"| Name | `{policy_set['name']}-<release>` | PolicySet identifier |")
                md.append(f"| Enabled | `{policy_set.get('enabled', False)}` | Whether this PolicySet is active |")
                
                if policy_set.get('policies'):
                    md.append("")
                    md.append("**Included Policies:**")
                    for p in policy_set['policies']:
                        md.append(f"- `{p}-<release>`")
                
                md.append("")
                md.append("---")
                md.append("")
        
        # Check for orphaned sub-policies
        orphaned = self._find_orphaned_policies(component)
        if orphaned['configs'] or orphaned['operators'] or orphaned['certificates']:
            md.append("## ⚠️ Warnings")
            md.append("")
            
            if orphaned['configs']:
                md.append("### Orphaned Configuration Policies")
                md.append("The following configuration policies reference disabled or non-existent parent policies:")
                for name in orphaned['configs']:
                    md.append(f"- {name}")
                md.append("")
            
            if orphaned['operators']:
                md.append("### Orphaned Operator Policies")
                md.append("The following operator policies reference disabled or non-existent parent policies:")
                for name in orphaned['operators']:
                    md.append(f"- {name}")
                md.append("")
            
            if orphaned['certificates']:
                md.append("### Orphaned Certificate Policies")
                md.append("The following certificate policies reference disabled or non-existent parent policies:")
                for name in orphaned['certificates']:
                    md.append(f"- {name}")
                md.append("")
        
        # Summary Statistics
        stats = self._calculate_statistics(component)
        md.append("## 📊 Summary")
        md.append("")
        md.append("| Resource Type | Count |")
        md.append("| ------------- | ----- |")
        md.append(f"| Policies | {stats['policies']} |")
        md.append(f"| Configuration Policies | {stats['configs']} |")
        md.append(f"| Operator Policies | {stats['operators']} |")
        md.append(f"| Certificate Policies | {stats['certificates']} |")
        md.append(f"| PolicySets | {stats['policySets']} |")
        total = sum(stats.values())
        md.append(f"| **Total Resources** | **{total}** |")
        
        return '\n'.join(md)
    
    def _generate_gating_section(self, md: List[str], sub: Dict, policy_name: str):
        """Render waitForOperator / extraDependencies / ignorePending for one sub-policy."""
        wait = sub.get('waitForOperator')
        extra = sub.get('extraDependencies')
        if not wait and not extra and not sub.get('ignorePending'):
            return
        md.append("**Gating:**")
        md.append("")
        if wait:
            names = [wait] if isinstance(wait, str) else list(wait)
            for n in names:
                md.append(f"- Waits for operator `{n}` to reach CSV phase `Succeeded`")
        if sub.get('ignorePending'):
            md.append("- Reports compliant while waiting (`ignorePending`)")
        if extra:
            md.append("")
            md.append("| Resolves To | Kind | Awaited State |")
            md.append("| ----------- | ---- | ------------- |")
            for dep in extra:
                md.append(self._format_dependency(dep, policy_name, 'ConfigurationPolicy'))
        md.append("")

    def _format_dependency(self, dep: Dict, owner: str, default_kind: str) -> str:
        """Render one dependency entry the way policy-library resolves it."""
        name = dep.get('name', '?')
        kind = dep.get('kind', default_kind)
        if dep.get('raw'):
            resolved = name
        elif kind == 'Policy':
            if dep.get('release'):
                resolved = f"{name}-{dep['release']}"
            elif dep.get('element'):
                resolved = f"{name}-{dep['element']}-<cluster>"
            else:
                resolved = f"{name}-<release>"
        else:
            resolved = f"{dep.get('policyRef', owner)}-{name}"
        return f"| `{resolved}` | `{kind}` | `{dep.get('compliance', 'Compliant')}` |"

    def _generate_policy_section(self, md: List[str], policy: Dict, component: Dict, 
                                 comments: Dict, path: List):
        """Generate documentation for a single policy and its sub-policies"""
        policy_name = policy['name']
        component_name = path[1]  # Extract component name from path
        
        md.append(f"### 📋 Policy: {policy_name}")
        
        # Get description - try by name first, then by index
        desc = (policy.get('description') or 
               self.get_field_description(comments, *path[:-1], policy_name) or
               self.get_field_description(comments, *path))
        if desc:
            md.append(f"> {desc}")
        
        md.append("")
        md.append("| Parameter | Value | Description |")
        md.append("| --------- | ----- | ----------- |")
        md.append(f"| Name | `{policy_name}-<release>` | Full policy name including release |")
        md.append(f"| Namespace | `<namespace>` | Policy namespace |")
        
        # Get field-specific descriptions
        enabled_desc = self.get_field_description(comments, *path, 'enabled') or "Whether this policy is templated"
        md.append(f"| Enabled | `{policy.get('enabled', False)}` | {enabled_desc} |")
        
        if policy.get('disabled'):
            desc = self.get_field_description(comments, *path, 'disabled') or "Whether policy is disabled in ACM"
            md.append(f"| Disabled (ACM) | `{policy['disabled']}` | {desc} |")
        
        if policy.get('severity'):
            desc = self.get_field_description(comments, *path, 'severity') or "Policy severity level"
            md.append(f"| Severity | `{policy['severity']}` | {desc} |")
        
        if policy.get('remediationAction'):
            desc = self.get_field_description(comments, *path, 'remediationAction') or "Action when policy is violated"
            md.append(f"| Remediation | `{policy['remediationAction']}` | {desc} |")
        
        md.append("")
        
        # Dependencies hold the whole policy Pending until the targets report Compliant.
        if policy.get('dependencies'):
            md.append("#### Dependencies")
            md.append("")
            md.append("This policy stays `Pending` until every target below reports the "
                      "listed compliance state.")
            md.append("")
            md.append("| Resolves To | Kind | Awaited State |")
            md.append("| ----------- | ---- | ------------- |")
            for dep in policy['dependencies']:
                md.append(self._format_dependency(dep, policy_name, 'Policy'))
            md.append("")
        
        # Compliance Metadata
        if any([policy.get('categories'), policy.get('controls'), policy.get('standards'),
                component.get('default', {}).get('categories'),
                component.get('default', {}).get('controls'),
                component.get('default', {}).get('standards')]):
            md.append("#### Compliance Metadata")
            md.append("| Type | Values | Description |")
            md.append("| ---- | ------ | ----------- |")
            
            categories = policy.get('categories') or component.get('default', {}).get('categories')
            if categories:
                source = "" if policy.get('categories') else " (default)"
                desc = self.get_field_description(comments, *path, 'categories') or "Category classifications"
                md.append(f"| Categories | {', '.join(categories)}{source} | {desc} |")
            
            controls = policy.get('controls') or component.get('default', {}).get('controls')
            if controls:
                source = "" if policy.get('controls') else " (default)"
                desc = self.get_field_description(comments, *path, 'controls') or "Control mappings"
                md.append(f"| Controls | {', '.join(controls)}{source} | {desc} |")
            
            standards = policy.get('standards') or component.get('default', {}).get('standards')
            if standards:
                source = "" if policy.get('standards') else " (default)"
                desc = self.get_field_description(comments, *path, 'standards') or "Compliance standards"
                md.append(f"| Standards | {', '.join(standards)}{source} | {desc} |")
            
            md.append("")
        
        # Find associated sub-policies
        sub_policies = self._find_sub_policies(policy_name, component)
        
        if sub_policies['configs'] or sub_policies['operators'] or sub_policies['certificates']:
            md.append("#### Associated Sub-Policies")
            md.append("")
            
            # Configuration Policies
            if sub_policies['configs']:
                md.append("##### Configuration Policies")
                md.append("")
                for idx, config in enumerate(sub_policies['configs']):
                    # Find the actual index in the original list
                    actual_idx = None
                    for i, c in enumerate(component.get('configPolicies', [])):
                        if c.get('name') == config.get('name'):
                            actual_idx = i
                            break
                    if actual_idx is not None:
                        config_path = ['stack', component_name, 'configPolicies', actual_idx]
                        self._generate_config_policy_section(md, config, policy_name, comments, config_path)
            
            # Operator Policies
            if sub_policies['operators']:
                md.append("##### Operator Policies")
                md.append("")
                for idx, operator in enumerate(sub_policies['operators']):
                    # Find the actual index
                    actual_idx = None
                    for i, o in enumerate(component.get('operatorPolicies', [])):
                        if o.get('name') == operator.get('name'):
                            actual_idx = i
                            break
                    if actual_idx is not None:
                        operator_path = ['stack', component_name, 'operatorPolicies', actual_idx]
                        self._generate_operator_policy_section(md, operator, policy_name, comments, operator_path)
            
            # Certificate Policies
            if sub_policies['certificates']:
                md.append("##### Certificate Policies")
                md.append("")
                for idx, cert in enumerate(sub_policies['certificates']):
                    # Find the actual index
                    actual_idx = None
                    for i, c in enumerate(component.get('certificatePolicies', [])):
                        if c.get('name') == cert.get('name'):
                            actual_idx = i
                            break
                    if actual_idx is not None:
                        cert_path = ['stack', component_name, 'certificatePolicies', actual_idx]
                        self._generate_certificate_policy_section(md, cert, policy_name, comments, cert_path)
        
        md.append("")
        md.append("---")
        md.append("")
    
    def _generate_config_policy_section(self, md: List[str], config: Dict, policy_name: str, 
                                       comments: Dict, path: List):
        """Generate documentation for a configuration policy"""
        config_name = config['name']
        md.append(f"###### ⚙️ Config: {config_name}")
        
        # Get description by name or index
        desc = (config.get('description') or 
               self.get_field_description(comments, *path[:-1], config_name) or
               self.get_field_description(comments, *path))
        if desc:
            md.append(f"> {desc}")
        
        md.append("")
        md.append("**Basic Configuration:**")
        md.append("| Parameter | Value | Description |")
        md.append("| --------- | ----- | ----------- |")
        md.append(f"| Name | `{policy_name}-{config_name}` | Configuration policy identifier |")
        
        compliance_desc = self.get_field_description(comments, *path, 'complianceType') or "Compliance requirement type"
        md.append(f"| Compliance Type | `{config.get('complianceType', 'musthave')}` | {compliance_desc} |")
        
        remediation_desc = self.get_field_description(comments, *path, 'remediationAction') or "Remediation action"
        md.append(f"| Remediation | `{config.get('remediationAction', 'inform')}` | {remediation_desc} |")
        
        severity_desc = self.get_field_description(comments, *path, 'severity') or "Severity level"
        md.append(f"| Severity | `{config.get('severity', 'low')}` | {severity_desc} |")
        
        if config.get('disableTemplating'):
            desc = self.get_field_description(comments, *path, 'disableTemplating') or "Template processing status"
            md.append(f"| Template Processing | Disabled | {desc} |")
        
        if config.get('rawTemplate'):
            desc = self.get_field_description(comments, *path, 'rawTemplate') or "Emitted under object-templates-raw"
            md.append(f"| Raw Template | Enabled | {desc} |")
        
        md.append("")
        
        self._generate_gating_section(md, config, policy_name)
        
        # Template Names with descriptions
        if config.get('templateNames'):
            md.append("**Templates:**")
            md.append("| Template File | Compliance Type | Description |")
            md.append("| ------------- | --------------- | ----------- |")
            
            for t_idx, template in enumerate(config['templateNames']):
                if isinstance(template, dict):
                    name = template.get('name', 'unknown')
                    compliance = template.get('complianceType', 'inherited')
                    # Get description for this specific template
                    template_desc = (self.get_field_description(comments, *path, 'templateNames', name) or
                                   self.get_field_description(comments, *path, 'templateNames', t_idx) or
                                   self.get_field_description(comments, *path, 'templateNames', t_idx, 'name') or
                                   "Template configuration")
                else:
                    name = template
                    compliance = 'inherited'
                    template_desc = self.get_field_description(comments, *path, 'templateNames', t_idx) or "Template configuration"
                
                md.append(f"| `converters/{name}.yaml` | {compliance} | {template_desc} |")
            md.append("")
        
        # Template Parameters with descriptions
        if config.get('enableTemplateParameters') and config.get('templateParameters'):
            md.append("**Template Parameters:**")
            md.append("| Parameter | Value | Description |")
            md.append("| --------- | ----- | ----------- |")
            
            for key, value in config['templateParameters'].items():
                param_desc = self.get_field_description(comments, *path, 'templateParameters', key) or "Parameter value"
                md.append(f"| `{key}` | `{value}` | {param_desc} |")
            md.append("")
    
    def _generate_operator_policy_section(self, md: List[str], operator: Dict, policy_name: str, 
                                         comments: Dict, path: List):
        """Generate documentation for an operator policy"""
        operator_name = operator['name']
        md.append(f"###### 🔧 Operator: {operator_name}")
        
        # Get description
        desc = (operator.get('description') or 
               self.get_field_description(comments, *path[:-1], operator_name) or
               self.get_field_description(comments, *path))
        if desc:
            md.append(f"> {desc}")
        
        md.append("")
        md.append("**Basic Configuration:**")
        md.append("| Parameter | Value | Description |")
        md.append("| --------- | ----- | ----------- |")
        md.append(f"| Name | `{policy_name}-{operator_name}` | Operator policy identifier |")
        
        namespace_desc = self.get_field_description(comments, *path, 'namespace') or "Target namespace for operator"
        md.append(f"| Namespace | `{operator.get('namespace', 'default')}` | {namespace_desc} |")
        
        display_desc = self.get_field_description(comments, *path, 'displayName') or "Display name for operator"
        md.append(f"| Display Name | `{operator.get('displayName', operator.get('subscription', {}).get('name', 'N/A'))}` | {display_desc} |")
        
        compliance_desc = self.get_field_description(comments, *path, 'complianceType') or "Compliance requirement"
        md.append(f"| Compliance Type | `{operator.get('complianceType', 'musthave')}` | {compliance_desc} |")
        
        remediation_desc = self.get_field_description(comments, *path, 'remediationAction') or "Remediation action"
        md.append(f"| Remediation | `{operator.get('remediationAction', 'inform')}` | {remediation_desc} |")
        
        severity_desc = self.get_field_description(comments, *path, 'severity') or "Severity level"
        md.append(f"| Severity | `{operator.get('severity', 'low')}` | {severity_desc} |")
        
        if operator.get('upgradeApproval'):
            upgrade_desc = self.get_field_description(comments, *path, 'upgradeApproval') or "Upgrade approval strategy"
            md.append(f"| Upgrade Approval | `{operator['upgradeApproval']}` | {upgrade_desc} |")
        
        md.append("")
        
        # Subscription Details with descriptions
        if operator.get('subscription'):
            sub = operator['subscription']
            md.append("**Subscription Details:**")
            md.append("| Parameter | Value | Description |")
            md.append("| --------- | ----- | ----------- |")
            
            name_desc = self.get_field_description(comments, *path, 'subscription', 'name') or "Operator package name"
            md.append(f"| Name | `{sub.get('name', 'N/A')}` | {name_desc} |")
            
            channel_desc = self.get_field_description(comments, *path, 'subscription', 'channel') or "Update channel"
            md.append(f"| Channel | `{sub.get('channel', 'N/A')}` | {channel_desc} |")
            
            source_desc = self.get_field_description(comments, *path, 'subscription', 'source') or "Catalog source"
            md.append(f"| Source | `{sub.get('source', 'N/A')}` | {source_desc} |")
            
            source_ns_desc = self.get_field_description(comments, *path, 'subscription', 'sourceNamespace') or "Catalog namespace"
            md.append(f"| Source Namespace | `{sub.get('sourceNamespace', 'N/A')}` | {source_ns_desc} |")
            
            if sub.get('startingCSV'):
                csv_desc = self.get_field_description(comments, *path, 'subscription', 'startingCSV') or "Initial operator version"
                md.append(f"| Starting CSV | `{sub['startingCSV']}` | {csv_desc} |")
            
            md.append("")
        
        # Approved Versions with descriptions
        if operator.get('versions'):
            versions_desc = self.get_field_description(comments, *path, 'versions') or ""
            md.append("**Approved Versions:**")
            if versions_desc:
                md.append(f"*{versions_desc}*")
            for v_idx, version in enumerate(operator['versions']):
                version_desc = self.get_field_description(comments, *path, 'versions', v_idx) or ""
                if version_desc:
                    md.append(f"- `{version}` - {version_desc}")
                else:
                    md.append(f"- `{version}`")
            md.append("")
        
        self._generate_gating_section(md, operator, policy_name)
    
    def _generate_certificate_policy_section(self, md: List[str], cert: Dict, policy_name: str, 
                                            comments: Dict, path: List):
        """Generate documentation for a certificate policy"""
        cert_name = cert['name']
        md.append(f"###### 🔐 Certificate: {cert_name}")
        
        # Get description
        desc = (cert.get('description') or 
               self.get_field_description(comments, *path[:-1], cert_name) or
               self.get_field_description(comments, *path))
        if desc:
            md.append(f"> {desc}")
        
        md.append("")
        md.append("**Basic Configuration:**")
        md.append("| Parameter | Value | Description |")
        md.append("| --------- | ----- | ----------- |")
        md.append(f"| Name | `{policy_name}-{cert_name}` | Certificate policy identifier |")
        
        remediation_desc = self.get_field_description(comments, *path, 'remediationAction') or "Remediation action"
        md.append(f"| Remediation | `{cert.get('remediationAction', 'inform')}` | {remediation_desc} |")
        
        severity_desc = self.get_field_description(comments, *path, 'severity') or "Severity level"
        md.append(f"| Severity | `{cert.get('severity', 'low')}` | {severity_desc} |")
        
        if cert.get('disableTemplating'):
            desc = self.get_field_description(comments, *path, 'disableTemplating') or "Template processing status"
            md.append(f"| Template Processing | Disabled | {desc} |")
        
        md.append("")
        
        # Duration Requirements with descriptions
        if any([cert.get('minimumDuration'), cert.get('minimumCADuration'),
                cert.get('maximumDuration'), cert.get('maximumCADuration')]):
            md.append("**Duration Requirements:**")
            
            min_desc = self.get_field_description(comments, *path, 'minimumDuration') or "hours"
            max_desc = self.get_field_description(comments, *path, 'maximumDuration') or "hours"
            min_ca_desc = self.get_field_description(comments, *path, 'minimumCADuration') or "hours"
            max_ca_desc = self.get_field_description(comments, *path, 'maximumCADuration') or "hours"
            
            md.append("| Type | Minimum | Maximum |")
            md.append("| ---- | ------- | ------- |")
            md.append(f"| Certificate | {cert.get('minimumDuration', '-')} {min_desc} | {cert.get('maximumDuration', '-')} {max_desc} |")
            md.append(f"| CA Certificate | {cert.get('minimumCADuration', '-')} {min_ca_desc} | {cert.get('maximumCADuration', '-')} {max_ca_desc} |")
            md.append("")
        
        # SAN Patterns with descriptions
        if cert.get('allowedSANPattern') or cert.get('disallowedSANPattern'):
            md.append("**SAN Patterns:**")
            if cert.get('allowedSANPattern'):
                pattern_desc = self.get_field_description(comments, *path, 'allowedSANPattern') or "Regex pattern for allowed SANs"
                md.append(f"- Allowed Pattern: `{cert['allowedSANPattern']}` - {pattern_desc}")
            if cert.get('disallowedSANPattern'):
                pattern_desc = self.get_field_description(comments, *path, 'disallowedSANPattern') or "Regex pattern for disallowed SANs"
                md.append(f"- Disallowed Pattern: `{cert['disallowedSANPattern']}` - {pattern_desc}")
            md.append("")
    
        
        self._generate_gating_section(md, cert, policy_name)
    
    def _find_sub_policies(self, policy_name: str, component: Dict) -> Dict[str, List]:
        """Find all sub-policies associated with a policy"""
        result = {
            'configs': [],
            'operators': [],
            'certificates': []
        }
        
        # Find configuration policies
        for config in component.get('configPolicies', []):
            if config.get('enabled') and config.get('policyRef') == policy_name:
                result['configs'].append(config)
        
        # Find operator policies
        for operator in component.get('operatorPolicies', []):
            if operator.get('enabled') and operator.get('policyRef') == policy_name:
                result['operators'].append(operator)
        
        # Find certificate policies
        for cert in component.get('certificatePolicies', []):
            if cert.get('enabled') and cert.get('policyRef') == policy_name:
                result['certificates'].append(cert)
        
        return result
    
    def _find_orphaned_policies(self, component: Dict) -> Dict[str, List]:
        """Find sub-policies that reference non-existent or disabled parent policies"""
        result = {
            'configs': [],
            'operators': [],
            'certificates': []
        }
        
        # Get list of enabled policies
        enabled_policies = {p['name'] for p in component.get('policies', []) if p.get('enabled')}
        
        # Check configuration policies
        for config in component.get('configPolicies', []):
            if config.get('enabled') and config.get('policyRef'):
                if config['policyRef'] not in enabled_policies:
                    result['configs'].append(config['name'])
        
        # Check operator policies
        for operator in component.get('operatorPolicies', []):
            if operator.get('enabled') and operator.get('policyRef'):
                if operator['policyRef'] not in enabled_policies:
                    result['operators'].append(operator['name'])
        
        # Check certificate policies
        for cert in component.get('certificatePolicies', []):
            if cert.get('enabled') and cert.get('policyRef'):
                if cert['policyRef'] not in enabled_policies:
                    result['certificates'].append(cert['name'])
        
        return result
    
    def _calculate_statistics(self, component: Dict) -> Dict[str, int]:
        """Calculate statistics for enabled resources"""
        return {
            'policies': sum(1 for p in component.get('policies', []) if p.get('enabled')),
            'configs': sum(1 for p in component.get('configPolicies', []) if p.get('enabled')),
            'operators': sum(1 for p in component.get('operatorPolicies', []) if p.get('enabled')),
            'certificates': sum(1 for p in component.get('certificatePolicies', []) if p.get('enabled')),
            'policySets': sum(1 for p in component.get('policySets', []) if p.get('enabled'))
        }
    
    def check_all_docs(self) -> int:
        """Check if all documentation is up to date"""
        if not self.stack_dir.exists():
            print(f"Error: Stack directory '{self.stack_dir}' does not exist")
            return 1
        
        docs_outdated = False
        changes_needed = []
        
        # Process each directory in the stack
        for element_path in self.stack_dir.iterdir():
            if element_path.is_dir() and not element_path.name.startswith('.'):
                # Generate documentation content
                doc_content = self.generate_element_docs(element_path)
                
                if doc_content:
                    output_file = self.output_dir / f"{element_path.name}.md"
                    
                    # Check if file exists
                    if not output_file.exists():
                        print(f"❌ Missing: {output_file}")
                        changes_needed.append(f"Missing: {element_path.name}.md")
                        docs_outdated = True
                    else:
                        # Read existing content
                        with open(output_file, 'r') as f:
                            existing_content = f.read()
                        
                        # Compare ignoring timestamps
                        if not self.compare_content(existing_content, doc_content):
                            print(f"❌ Outdated: {output_file}")
                            changes_needed.append(f"Outdated: {element_path.name}.md")
                            docs_outdated = True
                        else:
                            print(f"✓ Current: {output_file}")
        
        # Check index file
        elements = []
        for element_path in self.stack_dir.iterdir():
            if element_path.is_dir() and not element_path.name.startswith('.'):
                values_file = element_path / "values.yaml"
                if values_file.exists():
                    # Load values to check if there's valid content
                    with open(values_file, 'r') as f:
                        values = yaml.safe_load(f) or {}
                    component_name = self.to_camel_case(element_path.name)
                    if values.get('stack', {}).get(component_name):
                        elements.append(element_path.name)
        
        index_content = self._generate_index_content(sorted(elements))
        index_file = self.output_dir / "README.md"
        
        if not index_file.exists():
            print(f"❌ Missing: {index_file}")
            changes_needed.append("Missing: README.md")
            docs_outdated = True
        else:
            with open(index_file, 'r') as f:
                existing_index = f.read()
            
            if not self.compare_content(existing_index, index_content):
                print(f"❌ Outdated: {index_file}")
                changes_needed.append("Outdated: README.md")
                docs_outdated = True
            else:
                print(f"✓ Current: {index_file}")
        
        # Print summary
        print("\n" + "="*50)
        if docs_outdated:
            print("📚 Documentation Status: OUTDATED")
            print("\nFiles needing updates:")
            for change in changes_needed:
                print(f"  - {change}")
            print("\nRun 'python tools/doc-generator.py' to update documentation")
            return 1
        else:
            print("📚 Documentation Status: UP TO DATE")
            print("All documentation is current (ignoring timestamp changes)")
            return 0
    
    def _generate_index_content(self, elements: List[str]) -> str:
        """Generate index file content"""
        lines = []
        lines.append("# PolicyStack Documentation Index")
        lines.append("")
        lines.append(f"*Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}*")
        lines.append("")
        lines.append("## Available Elements")
        lines.append("")
        
        if elements:
            for element in elements:
                lines.append(f"- [{element}](./{element}.md)")
        else:
            lines.append("No elements documented yet.")
        
        lines.append("")
        lines.append("## Comment Notation Guide")
        lines.append("")
        lines.append("Use special comment notation in values.yaml files to add descriptions at any level:")
        lines.append("")
        lines.append("### Basic Usage")
        lines.append("")
        lines.append("```yaml")
        lines.append("# @description: This policy enforces security standards")
        lines.append("security-policy:")
        lines.append("  enabled: true")
        lines.append("```")
        lines.append("")
        
        lines.append("### Nested Field Descriptions")
        lines.append("")
        lines.append("```yaml")
        lines.append("configPolicies:")
        lines.append("  - name: example-config")
        lines.append("    # @desc: Whether to actually apply this configuration")
        lines.append("    enabled: true")
        lines.append("    ")
        lines.append("    # @description: Individual template configurations")
        lines.append("    templateNames:")
        lines.append("      # @desc: Network policy template for namespace isolation")
        lines.append("      - name: network-policy")
        lines.append("        complianceType: musthave")
        lines.append("      ")
        lines.append("      # @desc: RBAC template for role bindings")
        lines.append("      - name: rbac-config")
        lines.append("        complianceType: musthave")
        lines.append("    ")
        lines.append("    # @description: Template parameters with specific values")
        lines.append("    templateParameters:")
        lines.append("      # @desc: The namespace to apply policies to")
        lines.append("      targetNamespace: production")
        lines.append("      ")
        lines.append("      # @desc: Severity level for alerts (low/medium/high/critical)")
        lines.append("      alertLevel: high")
        lines.append("```")
        lines.append("")
        
        lines.append("### Array Item Descriptions")
        lines.append("")
        lines.append("```yaml")
        lines.append("operatorPolicies:")
        lines.append("  # @description: GitOps operator for continuous deployment")
        lines.append("  - name: openshift-gitops")
        lines.append("    enabled: true")
        lines.append("    ")
        lines.append("    # @desc: Which approved versions can be installed")
        lines.append("    versions:")
        lines.append("      # @desc: Initial stable release")
        lines.append("      - gitops-operator.v1.5.0")
        lines.append("      # @desc: Security patch release")
        lines.append("      - gitops-operator.v1.5.1")
        lines.append("      # @desc: Feature update with performance improvements")
        lines.append("      - gitops-operator.v1.6.0")
        lines.append("```")
        lines.append("")
        
        lines.append("## Notes")
        lines.append("")
        lines.append("- Place `@description:` or `@desc:` comments on the line immediately before the field")
        lines.append("- Descriptions work at any nesting level")
        lines.append("- Array items can be documented by placing the comment before the item")
        lines.append("- Both `@description:` and `@desc:` are supported (they're equivalent)")
        
        return '\n'.join(lines)
    
    def generate_all_docs(self):
        """Generate documentation for all elements in the stack"""
        if not self.stack_dir.exists():
            print(f"Error: Stack directory '{self.stack_dir}' does not exist")
            return
        
        generated = []
        
        # Process each directory in the stack
        for element_path in self.stack_dir.iterdir():
            if element_path.is_dir() and not element_path.name.startswith('.'):
                print(f"Processing element: {element_path.name}")
                
                doc_content = self.generate_element_docs(element_path)
                
                if doc_content:
                    # Write documentation file
                    output_file = self.output_dir / f"{element_path.name}.md"
                    with open(output_file, 'w') as f:
                        f.write(doc_content)
                    
                    generated.append(element_path.name)
                    print(f"  ✓ Generated documentation: {output_file}")
                else:
                    print(f"  ⚠ No valid configuration found for {element_path.name}")
        
        # Generate index file
        self.generate_index(generated)
        
        print(f"\n✅ Documentation generation complete!")
        print(f"   Generated {len(generated)} documentation files in '{self.output_dir}'")
    
    def generate_index(self, elements: List[str]):
        """Generate an index file listing all documented elements"""
        index_file = self.output_dir / "README.md"
        content = self._generate_index_content(sorted(elements))
        
        with open(index_file, 'w') as f:
            f.write(content)


def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(
        description="Generate documentation for PolicyStack elements",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Generate docs with default settings
  python doc-generator.py
  
  # Check if documentation is up to date
  python doc-generator.py --check
  
  # Specify custom directories
  python doc-generator.py --stack-dir ./stack --output-dir ./documentation
  
  # Generate docs for a specific element
  python doc-generator.py --element sample-element
        """
    )
    
    parser.add_argument(
        '--stack-dir',
        default='stack',
        help='Directory containing stack elements (default: stack)'
    )
    
    parser.add_argument(
        '--output-dir',
        default='docs',
        help='Output directory for documentation (default: docs)'
    )
    
    parser.add_argument(
        '--element',
        help='Generate documentation for a specific element only'
    )
    
    parser.add_argument(
        '--check',
        action='store_true',
        help='Check if documentation is up to date (returns exit code 1 if outdated)'
    )
    
    args = parser.parse_args()
    
    generator = DocumentationGenerator(
        stack_dir=args.stack_dir,
        output_dir=args.output_dir,
        check_mode=args.check
    )
    
    if args.check:
        # Check mode - verify docs are up to date
        return generator.check_all_docs()
    elif args.element:
        # Generate docs for specific element
        element_path = Path(args.stack_dir) / args.element
        if not element_path.exists():
            print(f"Error: Element '{args.element}' not found in {args.stack_dir}")
            return 1
        
        doc_content = generator.generate_element_docs(element_path)
        if doc_content:
            output_file = Path(args.output_dir) / f"{args.element}.md"
            output_file.parent.mkdir(exist_ok=True)
            with open(output_file, 'w') as f:
                f.write(doc_content)
            print(f"✓ Generated documentation: {output_file}")
        else:
            print(f"No valid configuration found for {args.element}")
            return 1
    else:
        # Generate docs for all elements
        generator.generate_all_docs()
    
    return 0


if __name__ == "__main__":
    exit(main())
