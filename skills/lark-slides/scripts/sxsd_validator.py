#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Internal XSD model and constraint validation for the Slides lint entrypoint."""

from __future__ import annotations

import math
import re
import xml.etree.ElementTree as ET
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from functools import lru_cache
from pathlib import Path
from typing import Any


XS_NS = "{http://www.w3.org/2001/XMLSchema}"
SML_NAMESPACE = "https://www.larkoffice.com/sml/2.0"
SML_LEGACY_HTTP_NAMESPACE = "http://www.larkoffice.com/sml/2.0"
SML_READBACK_NAMESPACE = "/sml/2.0"
ACCEPTED_SML_NAMESPACES = frozenset(
    (SML_NAMESPACE, SML_LEGACY_HTTP_NAMESPACE, SML_READBACK_NAMESPACE)
)


def local_name(value: str) -> str:
    if value.startswith("{"):
        return value.rsplit("}", 1)[-1]
    return value.rsplit(":", 1)[-1]


def direct_children(element: ET.Element, name: str) -> list[ET.Element]:
    return [child for child in element if child.tag == f"{XS_NS}{name}"]


def first_direct_child(element: ET.Element, *names: str) -> ET.Element | None:
    wanted = {f"{XS_NS}{name}" for name in names}
    return next((child for child in element if child.tag in wanted), None)


def occurs_value(raw: str | None, default: int) -> int | None:
    if raw == "unbounded":
        return None
    return int(raw) if raw is not None else default


@dataclass(frozen=True)
class SimpleTypeRule:
    name: str
    base: str | None = None
    enums: tuple[str, ...] = ()
    patterns: tuple[str, ...] = ()
    bounds: tuple[tuple[str, Decimal], ...] = ()
    length_bounds: tuple[tuple[str, int], ...] = ()
    union_members: tuple[str, ...] = ()


@dataclass(frozen=True)
class AttributeRule:
    name: str
    type_name: str
    required: bool


@dataclass(frozen=True)
class ElementRule:
    name: str
    type_name: str | None
    inline_complex_type: ET.Element | None
    ref_name: str | None


@dataclass(frozen=True)
class ChildRule:
    element: ElementRule
    min_occurs: int
    max_occurs: int | None
    order: int | None


@dataclass(frozen=True)
class WildcardRule:
    namespace: str
    process_contents: str
    min_occurs: int
    max_occurs: int | None
    order: int | None


@dataclass(frozen=True)
class ChoiceRequirement:
    names: tuple[str, ...]
    min_occurs: int
    max_occurs: int | None


@dataclass(frozen=True)
class SchemaModel:
    simple_types: dict[str, SimpleTypeRule]
    complex_types: dict[str, ET.Element]
    element_candidates: dict[str, tuple[ElementRule, ...]]
    global_elements: dict[str, ElementRule]


def parse_simple_type(
    element: ET.Element,
    fallback_name: str,
    simple_types: dict[str, SimpleTypeRule] | None = None,
) -> SimpleTypeRule:
    name = element.attrib.get("name", fallback_name)
    restriction = first_direct_child(element, "restriction")
    union = first_direct_child(element, "union")
    if union is not None:
        union_members = [
            local_name(member) for member in union.attrib.get("memberTypes", "").split()
        ]
        for index, inline_simple in enumerate(direct_children(union, "simpleType"), start=1):
            inline_name = f"__inline_union_member_{name}_{index}"
            union_members.append(inline_name)
            if simple_types is not None:
                simple_types[inline_name] = parse_simple_type(
                    inline_simple,
                    inline_name,
                    simple_types,
                )
        return SimpleTypeRule(
            name=name,
            union_members=tuple(union_members),
        )
    if restriction is None:
        return SimpleTypeRule(name=name)

    facet_names = {
        "minInclusive",
        "minExclusive",
        "maxInclusive",
        "maxExclusive",
    }
    bounds: list[tuple[str, Decimal]] = []
    length_bounds: list[tuple[str, int]] = []
    for child in restriction:
        facet = local_name(child.tag)
        if "value" not in child.attrib:
            continue
        if facet in facet_names:
            bounds.append((facet, Decimal(child.attrib["value"])))
        elif facet in {"minLength", "maxLength"}:
            length_bounds.append((facet, int(child.attrib["value"])))
    return SimpleTypeRule(
        name=name,
        base=local_name(restriction.attrib.get("base", "string")),
        enums=tuple(child.attrib["value"] for child in direct_children(restriction, "enumeration")),
        patterns=tuple(child.attrib["value"] for child in direct_children(restriction, "pattern")),
        bounds=tuple(bounds),
        length_bounds=tuple(length_bounds),
    )


def parse_element_rule(element: ET.Element) -> ElementRule | None:
    raw_ref = element.attrib.get("ref")
    name = element.attrib.get("name")
    if name is None and raw_ref:
        name = local_name(raw_ref)
    if not name:
        return None
    return ElementRule(
        name=name,
        type_name=local_name(element.attrib["type"]) if element.attrib.get("type") else None,
        inline_complex_type=first_direct_child(element, "complexType"),
        ref_name=local_name(raw_ref) if raw_ref else None,
    )


@lru_cache(maxsize=4)
def load_schema_model(schema_path: str) -> SchemaModel:
    root = ET.parse(schema_path).getroot()
    simple_types: dict[str, SimpleTypeRule] = {}
    for element in direct_children(root, "simpleType"):
        name = element.attrib.get("name")
        if not name:
            continue
        simple_types[name] = parse_simple_type(element, name, simple_types)
    for attribute in root.iter(f"{XS_NS}attribute"):
        inline_simple = first_direct_child(attribute, "simpleType")
        if inline_simple is None:
            continue
        inline_name = f"__inline_attribute_{attribute.attrib.get('name', 'anonymous')}_{id(attribute)}"
        simple_types[inline_name] = parse_simple_type(inline_simple, inline_name, simple_types)
    complex_types = {
        element.attrib["name"]: element
        for element in direct_children(root, "complexType")
        if element.attrib.get("name")
    }
    candidates: dict[str, list[ElementRule]] = {}
    for element in root.iter(f"{XS_NS}element"):
        rule = parse_element_rule(element)
        if rule is not None:
            candidates.setdefault(rule.name, []).append(rule)
    global_elements = {
        rule.name: rule
        for element in direct_children(root, "element")
        if (rule := parse_element_rule(element)) is not None
    }
    return SchemaModel(
        simple_types=simple_types,
        complex_types=complex_types,
        element_candidates={name: tuple(rules) for name, rules in candidates.items()},
        global_elements=global_elements,
    )


def attributes_for_complex_type(
    complex_type: ET.Element,
    model: SchemaModel,
    resolving: set[str] | None = None,
) -> dict[str, AttributeRule]:
    resolving = resolving or set()
    attributes: dict[str, AttributeRule] = {}

    for content_name in ("simpleContent", "complexContent"):
        content = first_direct_child(complex_type, content_name)
        if content is None:
            continue
        extension = first_direct_child(content, "extension")
        if extension is None:
            continue
        base_name = local_name(extension.attrib.get("base", ""))
        if base_name in model.complex_types and base_name not in resolving:
            resolving.add(base_name)
            attributes.update(attributes_for_complex_type(model.complex_types[base_name], model, resolving))
            resolving.remove(base_name)
        attributes.update(direct_attribute_rules(extension))

    attributes.update(direct_attribute_rules(complex_type))
    return attributes


def direct_attribute_rules(element: ET.Element) -> dict[str, AttributeRule]:
    rules: dict[str, AttributeRule] = {}
    for attribute in direct_children(element, "attribute"):
        name = attribute.attrib.get("name")
        if not name:
            continue
        type_name = local_name(attribute.attrib.get("type", "string"))
        inline_simple = first_direct_child(attribute, "simpleType")
        if inline_simple is not None:
            type_name = f"__inline_attribute_{name}_{id(attribute)}"
        rules[name] = AttributeRule(
            name=name,
            type_name=type_name,
            required=attribute.attrib.get("use") == "required",
        )
    return rules


def attributes_for_element(rule: ElementRule, model: SchemaModel) -> dict[str, AttributeRule]:
    complex_type = rule.inline_complex_type
    if complex_type is None and rule.type_name in model.complex_types:
        complex_type = model.complex_types[rule.type_name]
    if complex_type is None:
        return {}
    return attributes_for_complex_type(complex_type, model)


def best_element_rule(element_name: str, model: SchemaModel) -> ElementRule | None:
    candidates = model.element_candidates.get(element_name, ())
    if not candidates:
        return None
    return max(
        candidates,
        key=lambda candidate: (
            candidate.type_name is not None or candidate.inline_complex_type is not None,
            len(attributes_for_element(candidate, model)),
        ),
    )


def concrete_element_rule(rule: ElementRule, model: SchemaModel) -> ElementRule:
    if rule.ref_name is not None:
        return model.global_elements.get(rule.ref_name, rule)
    if rule.type_name is not None or rule.inline_complex_type is not None:
        return rule
    candidate = best_element_rule(rule.name, model)
    return candidate or rule


def complex_type_for_element(rule: ElementRule, model: SchemaModel) -> ET.Element | None:
    rule = concrete_element_rule(rule, model)
    if rule.inline_complex_type is not None:
        return rule.inline_complex_type
    if rule.type_name is not None:
        return model.complex_types.get(rule.type_name)
    return None


def particle_for_complex_type(complex_type: ET.Element) -> ET.Element | None:
    particle = first_direct_child(complex_type, "sequence", "all", "choice")
    if particle is not None:
        return particle
    for content_name in ("simpleContent", "complexContent"):
        content = first_direct_child(complex_type, content_name)
        if content is None:
            continue
        extension = first_direct_child(content, "extension")
        if extension is not None:
            return first_direct_child(extension, "sequence", "all", "choice")
    return None


def multiplied_max(left: int | None, right: int | None) -> int | None:
    if left is None or right is None:
        return None
    return left * right


def child_rules_for_complex_type(
    complex_type: ET.Element,
) -> tuple[list[ChildRule], list[WildcardRule], list[ChoiceRequirement]]:
    particle = particle_for_complex_type(complex_type)
    if particle is None:
        return [], [], []

    rules: list[ChildRule] = []
    wildcard_rules: list[WildcardRule] = []
    requirements: list[ChoiceRequirement] = []
    next_order = 0

    def add_element(
        element: ET.Element,
        *,
        order: int | None,
        optional_by_choice: bool,
        max_multiplier: int | None,
    ) -> None:
        element_rule = parse_element_rule(element)
        if element_rule is None:
            return
        minimum = occurs_value(element.attrib.get("minOccurs"), 1) or 0
        maximum = occurs_value(element.attrib.get("maxOccurs"), 1)
        rules.append(
            ChildRule(
                element=element_rule,
                min_occurs=0 if optional_by_choice else minimum,
                max_occurs=multiplied_max(maximum, max_multiplier),
                order=order,
            )
        )

    def add_wildcard(
        wildcard: ET.Element,
        *,
        order: int | None,
        optional_by_choice: bool,
        max_multiplier: int | None,
    ) -> None:
        minimum = occurs_value(wildcard.attrib.get("minOccurs"), 1) or 0
        maximum = occurs_value(wildcard.attrib.get("maxOccurs"), 1)
        wildcard_rules.append(
            WildcardRule(
                namespace=wildcard.attrib.get("namespace", "##any"),
                process_contents=wildcard.attrib.get("processContents", "strict"),
                min_occurs=0 if optional_by_choice else minimum,
                max_occurs=multiplied_max(maximum, max_multiplier),
                order=order,
            )
        )

    def walk_group(
        group: ET.Element,
        *,
        ordered: bool,
        fixed_order: int | None = None,
        optional_by_choice: bool = False,
        max_multiplier: int | None = 1,
    ) -> None:
        nonlocal next_order
        kind = local_name(group.tag)
        group_min = occurs_value(group.attrib.get("minOccurs"), 1) or 0
        group_max = occurs_value(group.attrib.get("maxOccurs"), 1)
        effective_max = multiplied_max(max_multiplier, group_max)

        if kind == "choice":
            choice_order = fixed_order
            if choice_order is None and ordered:
                choice_order = next_order
                next_order += 1
            names: list[str] = []
            for child in group:
                child_kind = local_name(child.tag)
                if child_kind == "element":
                    parsed = parse_element_rule(child)
                    if parsed is not None:
                        names.append(parsed.name)
                    add_element(
                        child,
                        order=choice_order,
                        optional_by_choice=True,
                        max_multiplier=effective_max,
                    )
                elif child_kind == "any":
                    add_wildcard(
                        child,
                        order=choice_order,
                        optional_by_choice=True,
                        max_multiplier=effective_max,
                    )
                elif child_kind in {"sequence", "all", "choice"}:
                    walk_group(
                        child,
                        ordered=ordered,
                        fixed_order=choice_order,
                        optional_by_choice=True,
                        max_multiplier=effective_max,
                    )
            if names and (group_min > 0 or effective_max is not None):
                requirements.append(ChoiceRequirement(tuple(names), group_min, effective_max))
            return

        group_ordered = kind == "sequence"
        for child in group:
            child_kind = local_name(child.tag)
            if child_kind == "element":
                child_order = fixed_order
                if child_order is None and ordered and group_ordered:
                    child_order = next_order
                    next_order += 1
                add_element(
                    child,
                    order=child_order,
                    optional_by_choice=optional_by_choice or group_min == 0,
                    max_multiplier=effective_max,
                )
            elif child_kind == "any":
                child_order = fixed_order
                if child_order is None and ordered and group_ordered:
                    child_order = next_order
                    next_order += 1
                add_wildcard(
                    child,
                    order=child_order,
                    optional_by_choice=optional_by_choice or group_min == 0,
                    max_multiplier=effective_max,
                )
            elif child_kind in {"sequence", "all", "choice"}:
                walk_group(
                    child,
                    ordered=ordered and group_ordered,
                    fixed_order=fixed_order,
                    optional_by_choice=optional_by_choice or group_min == 0,
                    max_multiplier=effective_max,
                )

    walk_group(particle, ordered=local_name(particle.tag) == "sequence")
    return rules, wildcard_rules, requirements


def issue(
    code: str,
    path: str,
    tag: str,
    *,
    attr: str | None,
    expected: str,
    actual: Any,
    message: str,
    hint: str,
) -> dict[str, Any]:
    result: dict[str, Any] = {
        "level": "error",
        "code": code,
        "path": path,
        "tag": tag,
        "expected": expected,
        "actual": actual,
        "message": message,
        "hint": hint,
    }
    if attr is not None:
        result["attr"] = attr
    return result


def builtin_scalar_value(type_name: str, value: str) -> Decimal | str | bool:
    if type_name in {"string", "anyURI"}:
        return value
    if type_name == "boolean":
        if value not in {"true", "false", "1", "0"}:
            raise ValueError("expected boolean")
        return value in {"true", "1"}
    if type_name in {"integer", "positiveInteger", "nonNegativeInteger"}:
        if re.fullmatch(r"[+-]?\d+", value) is None:
            raise ValueError("expected integer")
        number = Decimal(value)
        if type_name == "positiveInteger" and number <= 0:
            raise ArithmeticError("expected positive integer")
        if type_name == "nonNegativeInteger" and number < 0:
            raise ArithmeticError("expected non-negative integer")
        return number
    if type_name in {"double", "decimal"}:
        lexical_value = value.strip(" \t\n\r")
        decimal_pattern = r"[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)"
        double_pattern = decimal_pattern + r"(?:[eE][+-]?[0-9]+)?"
        expected_pattern = double_pattern if type_name == "double" else decimal_pattern
        if re.fullmatch(expected_pattern, lexical_value) is None:
            raise ValueError(f"expected {type_name}")
        try:
            number = Decimal(lexical_value)
        except InvalidOperation as error:
            raise ValueError(f"expected {type_name}") from error
        if not math.isfinite(float(number)):
            raise ValueError(f"expected finite {type_name}")
        return number
    return value


def scalar_value_for_type(
    type_name: str,
    value: str,
    model: SchemaModel,
    resolving: set[str] | None = None,
) -> Decimal | str | bool:
    resolving = resolving or set()
    if type_name in resolving:
        return value
    rule = model.simple_types.get(type_name)
    if rule is None or rule.base is None:
        return builtin_scalar_value(type_name, value)
    resolving.add(type_name)
    try:
        return scalar_value_for_type(rule.base, value, model, resolving)
    finally:
        resolving.remove(type_name)


@lru_cache(maxsize=32)
def python_pattern_for_xsd(pattern: str) -> str:
    translated: list[str] = []
    in_character_class = False
    index = 0
    while index < len(pattern):
        char = pattern[index]
        if char == "\\" and index + 1 < len(pattern):
            escaped = pattern[index + 1]
            if escaped in {"s", "S"}:
                body = r" \t\n\r"
                if in_character_class:
                    if escaped == "S":
                        raise ValueError(
                            f"unsupported complemented XSD character class \\{escaped} inside []"
                        )
                    translated.append(body)
                else:
                    prefix = "^" if escaped == "S" else ""
                    translated.append(f"[{prefix}{body}]")
                index += 2
                continue
            translated.extend((char, escaped))
            index += 2
            continue
        if char == "[":
            in_character_class = True
        elif char == "]":
            in_character_class = False
        elif char == "." and not in_character_class:
            translated.append(r"[^\n\r]")
            index += 1
            continue
        elif char in "^$" and not in_character_class:
            translated.append(f"\\{char}")
            index += 1
            continue
        translated.append(char)
        index += 1
    return "".join(translated)


def xsd_pattern_matches(pattern: str, value: str) -> bool:
    if pattern == r"[\w.-]+[.:]\S*":
        if any(character in " \t\n\r" for character in value):
            return False
        for index, character in enumerate(value):
            if index > 0 and character in ".:":
                return True
            if not (character == "_" or character.isalnum() or character in ".-"):
                return False
        return False
    return re.fullmatch(python_pattern_for_xsd(pattern), value) is not None


def value_error_for_type(
    type_name: str,
    value: str,
    model: SchemaModel,
    resolving: set[str] | None = None,
) -> tuple[str, str] | None:
    resolving = resolving or set()
    if type_name in resolving:
        return None
    rule = model.simple_types.get(type_name)
    if rule is None:
        try:
            builtin_scalar_value(type_name, value)
        except ValueError:
            return "sxsd_invalid_scalar", f"value valid for {type_name}"
        except ArithmeticError:
            return "sxsd_value_out_of_range", f"value in the range allowed by {type_name}"
        return None

    resolving.add(type_name)
    try:
        if rule.union_members:
            member_errors = [value_error_for_type(member, value, model, resolving) for member in rule.union_members]
            if any(error is None for error in member_errors):
                return None
            unsupported_error = next(
                (error for error in member_errors if error and error[0] == "sxsd_unsupported_pattern"),
                None,
            )
            if unsupported_error is not None:
                return unsupported_error
            if any(error and error[0] == "sxsd_pattern_mismatch" for error in member_errors):
                return "sxsd_pattern_mismatch", f"value matching one member of {type_name}"
            return member_errors[0]

        if rule.enums and value not in rule.enums:
            return "sxsd_invalid_enum", "one of: " + ", ".join(rule.enums)

        if rule.patterns:
            unsupported_patterns: list[str] = []
            for pattern in rule.patterns:
                try:
                    if xsd_pattern_matches(pattern, value):
                        break
                except (ValueError, re.error) as error:
                    unsupported_patterns.append(f"{pattern!r}: {error}")
            else:
                if unsupported_patterns:
                    return (
                        "sxsd_unsupported_pattern",
                        "lint support for XSD pattern " + "; ".join(unsupported_patterns),
                    )
                return "sxsd_pattern_mismatch", "value matching pattern " + " or ".join(rule.patterns)

        base_name = rule.base or "string"
        base_error = value_error_for_type(base_name, value, model, resolving)
        if base_error is not None:
            return base_error
        for facet, bound in rule.length_bounds:
            allowed = len(value) >= bound if facet == "minLength" else len(value) <= bound
            if not allowed:
                return "sxsd_value_out_of_range", f"{facet} {bound}"
        scalar = scalar_value_for_type(base_name, value, model)
        if isinstance(scalar, Decimal):
            for facet, bound in rule.bounds:
                allowed = {
                    "minInclusive": scalar >= bound,
                    "minExclusive": scalar > bound,
                    "maxInclusive": scalar <= bound,
                    "maxExclusive": scalar < bound,
                }[facet]
                if not allowed:
                    return "sxsd_value_out_of_range", f"{facet} {bound}"
        return None
    finally:
        resolving.remove(type_name)


def validate_element_attributes(
    element: ET.Element,
    path: str,
    model: SchemaModel,
    element_rule: ElementRule | None = None,
) -> list[dict[str, Any]]:
    tag = local_name(element.tag)
    element_rule = element_rule or best_element_rule(tag, model)
    if element_rule is None:
        return []
    attribute_rules = attributes_for_element(element_rule, model)
    issues: list[dict[str, Any]] = []
    for attr_rule in attribute_rules.values():
        if attr_rule.required and attr_rule.name not in element.attrib:
            issues.append(
                issue(
                    "sxsd_missing_required_attr",
                    path,
                    tag,
                    attr=attr_rule.name,
                    expected=f"required attribute of type {attr_rule.type_name}",
                    actual=None,
                    message=f'missing required SXSD attribute "{attr_rule.name}" on <{tag}> at {path}',
                    hint=f'Add attribute "{attr_rule.name}" with a value valid for {attr_rule.type_name}.',
                )
            )
    for raw_name, value in element.attrib.items():
        attr_name = local_name(raw_name)
        attr_rule = attribute_rules.get(attr_name)
        if attr_rule is None:
            continue
        validation_error = value_error_for_type(attr_rule.type_name, value, model)
        if validation_error is None:
            continue
        code, expected = validation_error
        if code == "sxsd_unsupported_pattern":
            message = (
                f'unsupported SXSD pattern for attribute "{attr_name}" on <{tag}> at {path}'
            )
            hint = (
                f"Extend the SXSD pattern interpreter for {attr_rule.type_name}; "
                "do not treat this attribute value as validated."
            )
        else:
            message = (
                f'invalid SXSD value {value!r} for attribute "{attr_name}" on <{tag}> at {path}'
            )
            hint = f'Set attribute "{attr_name}" to a value valid for {attr_rule.type_name}.'
        issues.append(
            issue(
                code,
                path,
                tag,
                attr=attr_name,
                expected=expected,
                actual=value,
                message=message,
                hint=hint,
            )
        )
    return issues


def element_namespace(tag: str) -> str | None:
    if not tag.startswith("{"):
        return None
    return tag[1:].split("}", 1)[0]


def wildcard_matches_namespace(namespace_rule: str, namespace: str | None) -> bool:
    tokens = namespace_rule.split()
    if "##any" in tokens:
        return True
    if "##local" in tokens and namespace is None:
        return True
    if "##targetNamespace" in tokens and namespace == SML_NAMESPACE:
        return True
    if "##other" in tokens and namespace not in {None, SML_NAMESPACE}:
        return True
    return namespace in tokens


def wildcard_namespace_description(namespace_rule: str) -> str:
    if namespace_rule == "##any":
        return "any namespace"
    if namespace_rule == "##local":
        return "the local namespace"
    if namespace_rule == "##targetNamespace":
        return f"the target namespace {SML_NAMESPACE}"
    if namespace_rule == "##other":
        return "a namespace other than the SXSD target namespace"
    return f"namespace {namespace_rule}"


def validate_element_children(
    element: ET.Element,
    path: str,
    element_rule: ElementRule,
    model: SchemaModel,
) -> tuple[list[dict[str, Any]], dict[int, ElementRule]]:
    tag = local_name(element.tag)
    complex_type = complex_type_for_element(element_rule, model)
    child_rules, wildcard_rules, choice_requirements = (
        child_rules_for_complex_type(complex_type)
        if complex_type is not None
        else ([], [], [])
    )
    rules_by_name: dict[str, list[ChildRule]] = {}
    for child_rule in child_rules:
        rules_by_name.setdefault(child_rule.element.name, []).append(child_rule)

    issues: list[dict[str, Any]] = []
    matched: dict[int, ElementRule] = {}
    counts: dict[str, int] = {}
    wildcard_counts: dict[int, int] = {}
    latest_order = -1
    for child in element:
        child_name = local_name(child.tag)
        child_path = f"{path}/{child_name}"
        candidates = rules_by_name.get(child_name, [])
        wildcard_match = next(
            (
                (index, wildcard_rule)
                for index, wildcard_rule in enumerate(wildcard_rules)
                if wildcard_matches_namespace(
                    wildcard_rule.namespace,
                    element_namespace(child.tag),
                )
            ),
            None,
        )
        if not candidates and wildcard_match is None:
            expected_children = sorted(rules_by_name)
            expected_children.extend(
                wildcard_namespace_description(rule.namespace) for rule in wildcard_rules
            )
            issues.append(
                issue(
                    "sxsd_unexpected_child",
                    child_path,
                    child_name,
                    attr=None,
                    expected="one of: " + ", ".join(expected_children)
                    if expected_children
                    else "no child elements",
                    actual=child_name,
                    message=f"unexpected SXSD child <{child_name}> under <{tag}> at {child_path}",
                    hint=f"Move or remove <{child_name}> so <{tag}> follows the SXSD child structure.",
                )
            )
            continue

        if not candidates and wildcard_match is not None:
            wildcard_index, wildcard_rule = wildcard_match
            if wildcard_rule.order is not None:
                if wildcard_rule.order < latest_order:
                    issues.append(
                        issue(
                            "sxsd_invalid_child_order",
                            child_path,
                            child_name,
                            attr=None,
                            expected="children in xs:sequence order",
                            actual=child_name,
                            message=f"SXSD child <{child_name}> is out of order under <{tag}> at {child_path}",
                            hint=f"Reorder <{child_name}> according to the SXSD sequence for <{tag}>.",
                        )
                    )
                latest_order = max(latest_order, wildcard_rule.order)
            wildcard_counts[wildcard_index] = wildcard_counts.get(wildcard_index, 0) + 1
            actual_count = wildcard_counts[wildcard_index]
            if wildcard_rule.max_occurs is not None and actual_count > wildcard_rule.max_occurs:
                issues.append(
                    issue(
                        "sxsd_too_many_children",
                        child_path,
                        child_name,
                        attr=None,
                        expected=f"at most {wildcard_rule.max_occurs} child from {wildcard_namespace_description(wildcard_rule.namespace)}",
                        actual=actual_count,
                        message=f"too many wildcard SXSD children under <{tag}> at {path}",
                        hint=f"Keep at most {wildcard_rule.max_occurs} child from {wildcard_namespace_description(wildcard_rule.namespace)} under <{tag}>.",
                    )
                )
            continue

        child_rule = candidates[0]
        if child_rule.order is not None:
            if child_rule.order < latest_order:
                issues.append(
                    issue(
                        "sxsd_invalid_child_order",
                        child_path,
                        child_name,
                        attr=None,
                        expected="children in xs:sequence order",
                        actual=child_name,
                        message=f"SXSD child <{child_name}> is out of order under <{tag}> at {child_path}",
                        hint=f"Reorder <{child_name}> according to the SXSD sequence for <{tag}>.",
                    )
                )
            latest_order = max(latest_order, child_rule.order)

        counts[child_name] = counts.get(child_name, 0) + 1
        if child_rule.max_occurs is not None and counts[child_name] > child_rule.max_occurs:
            issues.append(
                issue(
                    "sxsd_too_many_children",
                    child_path,
                    child_name,
                    attr=None,
                    expected=f"at most {child_rule.max_occurs}",
                    actual=counts[child_name],
                    message=f"too many SXSD <{child_name}> children under <{tag}> at {path}",
                    hint=f"Keep at most {child_rule.max_occurs} <{child_name}> children under <{tag}>.",
                )
            )
        matched[id(child)] = concrete_element_rule(child_rule.element, model)

    for child_rule in child_rules:
        child_name = child_rule.element.name
        actual_count = counts.get(child_name, 0)
        if child_rule.min_occurs <= actual_count:
            continue
        issues.append(
            issue(
                "sxsd_missing_required_child",
                path,
                tag,
                attr=None,
                expected=f"{child_name} (at least {child_rule.min_occurs})",
                actual=actual_count,
                message=f"missing required SXSD child <{child_name}> under <{tag}> at {path}",
                hint=f"Add at least {child_rule.min_occurs} <{child_name}> child under <{tag}>.",
            )
        )

    for wildcard_index, wildcard_rule in enumerate(wildcard_rules):
        actual_count = wildcard_counts.get(wildcard_index, 0)
        if wildcard_rule.min_occurs <= actual_count:
            continue
        namespace_description = wildcard_namespace_description(wildcard_rule.namespace)
        issues.append(
            issue(
                "sxsd_missing_required_child",
                path,
                tag,
                attr=None,
                expected=f"at least {wildcard_rule.min_occurs} child from {namespace_description}",
                actual=actual_count,
                message=f"missing required wildcard SXSD child under <{tag}> at {path}",
                hint=f"Add at least {wildcard_rule.min_occurs} child from {namespace_description} under <{tag}>.",
            )
        )

    for requirement in choice_requirements:
        actual_count = sum(counts.get(name, 0) for name in requirement.names)
        expected_names = ", ".join(requirement.names)
        if actual_count < requirement.min_occurs:
            issues.append(
                issue(
                    "sxsd_missing_required_child",
                    path,
                    tag,
                    attr=None,
                    expected=f"one of: {expected_names} (at least {requirement.min_occurs})",
                    actual=actual_count,
                    message=f"missing required SXSD choice child under <{tag}> at {path}",
                    hint=f"Add at least {requirement.min_occurs} child from: {expected_names}.",
                )
            )
        if requirement.max_occurs is not None and actual_count > requirement.max_occurs:
            issues.append(
                issue(
                    "sxsd_too_many_children",
                    path,
                    tag,
                    attr=None,
                    expected=f"at most {requirement.max_occurs} child from: {expected_names}",
                    actual=actual_count,
                    message=f"too many SXSD choice children under <{tag}> at {path}",
                    hint=f"Keep at most {requirement.max_occurs} child from: {expected_names}.",
                )
            )
    return issues, matched


def validate_sxsd(root: ET.Element, schema_path: Path) -> list[dict[str, Any]]:
    model = load_schema_model(str(schema_path.resolve()))
    issues: list[dict[str, Any]] = []
    root_name = local_name(root.tag)
    document_namespace = element_namespace(root.tag)
    is_bare_slide_fragment = root_name == "slide" and document_namespace is None
    has_valid_document_namespace = (
        document_namespace in ACCEPTED_SML_NAMESPACES or is_bare_slide_fragment
    )

    def visit(element: ET.Element, parent_path: str, element_rule: ElementRule) -> None:
        tag = local_name(element.tag)
        path = f"{parent_path}/{tag}" if parent_path else tag
        namespace = element_namespace(element.tag)
        invalid_root_namespace = (
            not parent_path
            and namespace not in ACCEPTED_SML_NAMESPACES
            and not is_bare_slide_fragment
        )
        invalid_descendant_namespace = (
            bool(parent_path)
            and has_valid_document_namespace
            and namespace != document_namespace
        )
        if invalid_root_namespace or invalid_descendant_namespace:
            expected_namespace = document_namespace if parent_path else SML_NAMESPACE
            namespace_hint = (
                "Keep SXSD descendants without xmlns in a bare <slide> readback fragment."
                if expected_namespace is None
                else f'Use xmlns="{expected_namespace}" for SXSD elements.'
            )
            issues.append(
                issue(
                    "sxsd_invalid_namespace",
                    path,
                    tag,
                    attr=None,
                    expected=expected_namespace,
                    actual=namespace,
                    message=f"invalid SXSD namespace on <{tag}> at {path}",
                    hint=namespace_hint,
                )
            )
        issues.extend(validate_element_attributes(element, path, model, element_rule))
        child_issues, matched = validate_element_children(element, path, element_rule, model)
        issues.extend(child_issues)
        for child in element:
            child_rule = matched.get(id(child))
            if child_rule is not None:
                visit(child, path, child_rule)

    if root_name not in {"presentation", "slide"}:
        issues.append(
            issue(
                "sxsd_unexpected_root",
                root_name,
                root_name,
                attr=None,
                expected="presentation or slide",
                actual=root_name,
                message=f"unsupported SXSD root <{root_name}>",
                hint="Use a <presentation> or <slide> root.",
            )
        )
        return issues
    if root_name == "presentation":
        root_rule = model.global_elements.get("presentation")
    elif "SlideType" in model.complex_types:
        root_rule = ElementRule("slide", "SlideType", None, None)
    else:
        root_rule = None
    if root_rule is None:
        issues.append(
            issue(
                "sxsd_unexpected_root",
                root_name,
                root_name,
                attr=None,
                expected="presentation or slide",
                actual=root_name,
                message=f"unsupported SXSD root <{root_name}>",
                hint="Use a <presentation> or <slide> root.",
            )
        )
        return issues
    visit(root, "", root_rule)
    return issues


def load_tag_attributes(schema_path: Path) -> dict[str, set[str]]:
    model = load_schema_model(str(schema_path.resolve()))
    tag_attributes: dict[str, set[str]] = {}
    for tag_name, candidates in model.element_candidates.items():
        attrs = tag_attributes.setdefault(tag_name, set())
        for candidate in candidates:
            attrs.update(attributes_for_element(concrete_element_rule(candidate, model), model))
    return tag_attributes
