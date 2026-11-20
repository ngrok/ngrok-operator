# Closes the generated values schema, so a typo in a values path fails the
# render instead of being silently ignored. Run over values.schema.json after
# readme-generator, with values.yaml in $values: see the Makefile.
#
# readme-generator leaves every object open, and leaves out the keys documented
# with "@extra", which have no value in values.yaml for it to read. Those keys
# are put back first, then every object with a known key set is closed.
#
# An object with no "properties" is a free-form map (labels, annotations,
# extraEnv, resources) and stays open. So does the root: Helm keeps subchart
# values there, and templates/_legacy-values.tpl reports a pre-0.25 top-level
# key itself, naming the path it moved to.

# The schema path of a values path: a.b -> properties.a.properties.b
def at($path): reduce ($path | split(".")[]) as $key ([]; . + ["properties", $key]);

# Every "@extra" key, as {path, description}.
def extras: [$values | splits("\n") | capture("^## @extra +(?<path>[^ ]+) +(?<description>.*\\S)\\s*$")];

reduce extras[] as $extra (.;
  if getpath(at($extra.path) | .[:-1]) == null then
    error("values.yaml documents @extra \($extra.path), but the schema has no \($extra.path | split(".") | .[:-1] | join(".")) object")
  elif getpath(at($extra.path)) == null then
    # No type or default: the key has no chart default, and Kubernetes' own
    # applies until one is set.
    setpath(at($extra.path); {description: $extra.description})
  else . end)
| .properties |= walk(
    if type == "object" and has("properties")
    then . + {additionalProperties: false}
    else . end)
