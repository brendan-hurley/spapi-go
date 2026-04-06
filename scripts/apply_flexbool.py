#!/usr/bin/env python3
# ABOUTME: Post-generation step that replaces bool struct fields with flexbool.FlexBool
# ABOUTME: in generated model files to handle Amazon SP-API's string-encoded booleans.
"""
Amazon's SP-API specs declare many response fields as boolean, but the live API
sometimes returns them as JSON strings ("true" / "false"). The flexbool.FlexBool
type handles both representations transparently.

This script runs after openapi-generator and transforms every model_*.go file
that contains bool struct fields. It is designed to be idempotent.

Transformations applied:
  - Struct fields: *bool → *flexbool.FlexBool, bool → flexbool.FlexBool
  - Ok getter return types: (*bool, bool) → (*flexbool.FlexBool, bool)
  - Getters: return *o.Field → return bool(*o.Field)
  - Setters (optional): o.Field = &v → o.Field = flexbool.PtrFlexBool(v)
  - Setters (required): o.Field = v → o.Field = flexbool.FlexBool(v)
  - Constructors: this.Field = param → this.Field = flexbool.FlexBool(param)
  - Import: adds "github.com/brendan-hurley/spapi-go/flexbool"
"""
import re
import sys
from pathlib import Path

MODULE = "github.com/brendan-hurley/spapi-go"
FLEXBOOL_IMPORT = f'"{MODULE}/flexbool"'


def find_bool_fields(content):
    """Identify bool struct fields in a Go model file.

    Returns (optional_fields, required_fields) where:
      optional_fields = field names declared as *bool (pointer, omitempty)
      required_fields = field names declared as bool  (non-pointer, required)

    Only matches fields with `json:` struct tags (i.e. actual model fields,
    not internal/local variables).
    """
    optional = [
        m.group(1)
        for m in re.finditer(r"\t(\w+)\s+\*bool\s+`json:", content)
    ]
    required = [
        m.group(1)
        for m in re.finditer(r"\t(\w+)\s+bool\s+`json:", content)
    ]
    return optional, required


def add_flexbool_import(content):
    """Insert the flexbool import into the file's import block.

    Places it after stdlib imports, separated by a blank line (Go convention).
    No-op if the import is already present.
    """
    if FLEXBOOL_IMPORT in content:
        return content

    # Match the import block: all tab-indented lines between import( and )
    return re.sub(
        r"(import \(\n(?:\t[^\n]*\n)*)",
        r"\1\n\t" + FLEXBOOL_IMPORT + r"\n",
        content,
        count=1,
    )


def transform_optional_fields(content, fields):
    """Apply FlexBool transformations for optional (*bool) fields."""
    for field in fields:
        # Getter body: return *o.Field → return bool(*o.Field)
        content = content.replace(
            f"return *o.{field}\n",
            f"return bool(*o.{field})\n",
        )
        # Setter body: o.Field = &v → o.Field = flexbool.PtrFlexBool(v)
        content = content.replace(
            f"o.{field} = &v\n",
            f"o.{field} = flexbool.PtrFlexBool(v)\n",
        )
    return content


def transform_required_fields(content, fields):
    """Apply FlexBool transformations for required (non-pointer bool) fields."""
    for field in fields:
        escaped = re.escape(field)
        # Getter body: return o.Field → return bool(o.Field)
        content = re.sub(
            rf"(return) o\.{escaped}\n",
            rf"\1 bool(o.{field})\n",
            content,
        )
        # Setter body: o.Field = v → o.Field = flexbool.FlexBool(v)
        content = content.replace(
            f"o.{field} = v\n",
            f"o.{field} = flexbool.FlexBool(v)\n",
        )
        # Constructor body: this.Field = paramName → this.Field = flexbool.FlexBool(paramName)
        # The generated constructor parameter is camelCase (first letter lowered).
        param = field[0].lower() + field[1:]
        content = content.replace(
            f"this.{field} = {param}\n",
            f"this.{field} = flexbool.FlexBool({param})\n",
        )
    return content


def transform_model(filepath):
    """Apply all FlexBool transformations to a single model file.

    Returns True if the file was modified.
    """
    content = filepath.read_text()
    optional, required = find_bool_fields(content)

    if not optional and not required:
        return False

    # Add import
    content = add_flexbool_import(content)

    # Replace struct field types (before method transformations so field names
    # are found by the struct-field regex, not confused with changed code)
    content = re.sub(
        r"(\t\w+\s+)\*bool(\s+`json:)",
        r"\1*flexbool.FlexBool\2",
        content,
    )
    content = re.sub(
        r"(\t\w+)\s+bool(\s+`json:)",
        r"\1 flexbool.FlexBool\2",
        content,
    )

    # Replace Ok getter return types: (*bool, bool) → (*flexbool.FlexBool, bool)
    content = content.replace("(*bool, bool)", "(*flexbool.FlexBool, bool)")

    # Transform method bodies
    content = transform_optional_fields(content, optional)
    content = transform_required_fields(content, required)

    filepath.write_text(content)
    return True


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <apis_dir>", file=sys.stderr)
        sys.exit(1)

    apis_dir = Path(sys.argv[1])
    if not apis_dir.is_dir():
        print(f"Not a directory: {apis_dir}", file=sys.stderr)
        sys.exit(1)

    count = 0
    for model_file in sorted(apis_dir.rglob("model_*.go")):
        if transform_model(model_file):
            count += 1
            print(f"  flexbool: {model_file.relative_to(apis_dir)}")

    print(f">> flexbool: transformed {count} model files")


if __name__ == "__main__":
    main()
