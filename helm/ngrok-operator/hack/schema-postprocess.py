#!/usr/bin/env python3
"""Correct the schema readme-generator produces.

Two things the generator cannot express, both applied mechanically:

1. Every key under `defaults` is also settable per component. The generator
   only describes keys written out in values.yaml, and declaring the pod
   settings four times over is exactly the duplication this refactor removes,
   so the generated `defaults` properties are copied into each component.

2. The generator never emits additionalProperties: false, so an unrecognized
   value would be accepted silently. It is added to every object that has a
   `properties` key. The generator gives a nesting node `properties` and gives
   a free-form map `description` and `default` instead, so this seals the tree
   without sealing the maps users are meant to fill in.

A third thing the generator has no input for: `global`, `common` and
`ngrok-crds` are never written in values.yaml -- they're Helm's own umbrella
mechanism and the bitnamicharts/common and ngrok-crds subchart passthroughs --
but their subcharts' own default values still coalesce into the root of
`.Values` at render time. Sealing the root without describing them would
reject every install. They're added as free-form passthroughs, same shape as
any other free-form map in this schema.

A fourth correction: for a `[string,nullable]` / `[array,nullable]` key, the
generator writes OpenAPI-style `"type": "string", "nullable": true`. `helm
template`/`helm install` accept that extension, but the `helm unittest` plugin
validates with a plain JSON Schema library that doesn't know it, and fails
every null default with "got null, want string". Standard JSON Schema's own
way to say the same thing -- `"type": ["string", "null"]` -- is understood by
both, so every nullable key is normalized to that form.

Run by `make update-readme`. Never edit values.schema.json by hand.
"""

import copy
import json
import sys

# Hardcoded rather than derived from the schema: a fourth component would be
# silently unmirrored by mirror_defaults() -- it would still fail loudly at
# install (additionalProperties: false on that component's own node), so this
# is a note for future maintenance, not a defect.
COMPONENTS = ("apiManager", "agent", "bindingsForwarder")

SUBCHART_PASSTHROUGHS = {
    "global": (
        "Umbrella-chart globals. Helm copies a parent chart's `global` into "
        "every subchart's values, so this must be accepted for the chart to "
        "render as a dependency. Reserved for bitnamicharts/common "
        "passthrough; the operator reads nothing from it."
    ),
    "common": "Values passed through to the bitnamicharts/common dependency.",
    "ngrok-crds": "Values passed through to the ngrok-crds subchart.",
}


def add_subchart_passthroughs(properties):
    """Describe the subchart-owned keys that coalesce into the root."""
    for key, description in SUBCHART_PASSTHROUGHS.items():
        properties.setdefault(
            key, {"type": "object", "description": description, "default": {}}
        )


def mirror_defaults(properties):
    """Copy the generated `defaults` properties into each component."""
    defaults = properties.get("defaults", {}).get("properties")
    if not defaults:
        raise SystemExit("schema has no `defaults` properties to mirror")

    for component in COMPONENTS:
        node = properties.get(component)
        if node is None:
            raise SystemExit(f"schema has no `{component}` node")
        own = node.setdefault("properties", {})
        for key, value in defaults.items():
            own.setdefault(key, copy.deepcopy(value))


def seal(node):
    """Add additionalProperties: false to every object that has properties."""
    if not isinstance(node, dict):
        return

    if node.get("type") == "object" and "properties" in node:
        node["additionalProperties"] = False
        for child in node["properties"].values():
            seal(child)
    elif node.get("type") == "array" and isinstance(node.get("items"), dict):
        seal(node["items"])


def normalize_nullable(node):
    """Turn OpenAPI-style `nullable: true` into standard JSON Schema."""
    if not isinstance(node, dict):
        return

    if node.pop("nullable", False):
        node["type"] = [node.get("type", "string"), "null"]

    for child in node.get("properties", {}).values():
        normalize_nullable(child)
    items = node.get("items")
    if isinstance(items, dict):
        normalize_nullable(items)


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: schema-postprocess.py <values.schema.json>")

    path = sys.argv[1]
    with open(path) as f:
        schema = json.load(f)

    mirror_defaults(schema["properties"])
    add_subchart_passthroughs(schema["properties"])
    # seal() must run before normalize_nullable(): it matches nodes by
    # `type == "object"`/`"array"`, and normalize_nullable() turns a nullable
    # key's `type` into a list (`["string", "null"]`), which no longer equals
    # either string. Reversing the order would leave nullable objects/arrays
    # unsealed.
    seal(schema)
    normalize_nullable(schema)

    with open(path, "w") as f:
        json.dump(schema, f, indent=4)
        f.write("\n")


if __name__ == "__main__":
    main()
