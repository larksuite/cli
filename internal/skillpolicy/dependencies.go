// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillpolicy

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillManifest is the build-integrity metadata frozen when a skill tree is
// scanned. Runtime content within the owning skill directory remains live, but
// composition must not be able to change its dependency contract after Resolve.
type skillManifest struct {
	requiredSkills []string
}

func readSkillManifest(source fs.FS, name string) (skillManifest, error) {
	data, err := fs.ReadFile(source, name+"/SKILL.md")
	if err != nil {
		return skillManifest{}, fmt.Errorf("cannot read SKILL.md: %w", err)
	}
	required, err := parseRequiredSkills(name, data)
	if err != nil {
		return skillManifest{}, err
	}
	return skillManifest{requiredSkills: required}, nil
}

// parseRequiredSkills reads only the structured hard-dependency declaration:
//
//	metadata:
//	  requires:
//	    skills: ["lark-shared"]
//
// Markdown links and prose are intentionally irrelevant. A SKILL.md without
// YAML frontmatter declares no hard dependencies; malformed frontmatter that
// purports to be structured metadata fails closed during composition.
func parseRequiredSkills(skillName string, skillMD []byte) ([]string, error) {
	lines := strings.Split(string(skillMD), "\n")
	if strings.TrimRight(lines[0], "\r") != "---" {
		return nil, nil
	}

	block := make([]string, 0, len(lines))
	closed := false
	for _, line := range lines[1:] {
		if strings.TrimRight(line, "\r") == "---" {
			closed = true
			break
		}
		block = append(block, line)
	}
	if !closed {
		return nil, fmt.Errorf("SKILL.md frontmatter is not closed")
	}

	var frontmatter struct {
		Metadata struct {
			Requires struct {
				Skills []string `yaml:"skills"`
			} `yaml:"requires"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &frontmatter); err != nil {
		return nil, fmt.Errorf("cannot parse SKILL.md frontmatter: %w", err)
	}

	required := frontmatter.Metadata.Requires.Skills
	seen := make(map[string]struct{}, len(required))
	out := make([]string, 0, len(required))
	for _, dependency := range required {
		if !isSkillName(dependency) {
			return nil, fmt.Errorf("required skill %q declared by %q is not a valid skill name", dependency, skillName)
		}
		if _, duplicate := seen[dependency]; duplicate {
			continue
		}
		seen[dependency] = struct{}{}
		out = append(out, dependency)
	}
	return out, nil
}

// validateRequiredSkills checks the already-composed owner manifest. It must
// run after Base -> Allow -> Remove -> Overlay so no validation branch can
// accidentally disagree with the tree that list/read actually serves.
func validateRequiredSkills(composed *overlayFS) error {
	if composed == nil {
		return nil
	}
	names := make([]string, 0, len(composed.owner))
	for name := range composed.owner {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, dependency := range composed.owner[name].manifest.requiredSkills {
			if _, present := composed.owner[dependency]; !present {
				return fmt.Errorf("%w: skill %q requires skill %q, but %q is absent from the composed skill tree", ErrUnsatisfiedSkillDependency, name, dependency, dependency)
			}
		}
	}
	return nil
}
