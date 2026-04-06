#!/usr/bin/env python3
# ABOUTME: Post-generation step that replaces time.Time struct fields with flextime.FlexTime
# ABOUTME: in generated model files to handle Amazon SP-API's empty-string timestamps.
"""
Amazon's SP-API specs declare many response fields as date-time, but the live
API sometimes returns "" (empty string) for optional time fields. Go's
time.Time.UnmarshalJSON fails on empty strings. The flextime.FlexTime type
wraps time.Time and handles both valid timestamps and empty strings.

This script runs after openapi-generator and transforms every model_*.go file
that contains time.Time struct fields. It is designed to be idempotent.

Transformations applied:
  - Struct fields: *time.Time → *flextime.FlexTime, time.Time → flextime.FlexTime
  - Ok getter return types: (*time.Time, bool) → (*flextime.FlexTime, bool)
  - Getters (optional): return *o.Field → return o.Field.Time
  - Getters (required): return o.Field → return o.Field.Time
  - Setters (optional): o.Field = &v → o.Field = flextime.PtrFlexTime(v)
  - Setters (required): o.Field = v → o.Field = flextime.FlexTime{Time: v}
  - Constructors: this.Field = param → this.Field = flextime.FlexTime{Time: param}
  - Import: adds "github.com/brendan-hurley/spapi-go/flextime"
  - var ret time.Time stays as-is (zero value, getter still returns time.Time)
"""
import re
import sys
from pathlib import Path

MODULE = "github.com/brendan-hurley/spapi-go"
FLEXTIME_IMPORT = f'"{MODULE}/flextime"'


def find_time_fields(content):
    """Identify time.Time struct fields in a Go model file.

    Returns (optional_fields, required_fields) where:
      optional_fields = field names declared as *time.Time (pointer, omitempty)
      required_fields = field names declared as time.Time  (non-pointer, required)

    Only matches fields with `json:` struct tags.
    """
    optional = [
        m.group(1)
        for m in re.finditer(r"\t(\w+)\s+\*time\.Time\s+`json:", content)
    ]
    required = [
        m.group(1)
        for m in re.finditer(r"\t(\w+)\s+time\.Time\s+`json:", content)
    ]
    return optional, required


def add_flextime_import(content):
    """Insert the flextime import into the file's import block.

    Places it after stdlib imports, separated by a blank line.
    No-op if the import is already present.
    """
    if FLEXTIME_IMPORT in content:
        return content

    return re.sub(
        r"(import \(\n(?:\t[^\n]*\n)*)",
        r"\1\n\t" + FLEXTIME_IMPORT + r"\n",
        content,
        count=1,
    )


def transform_optional_fields(content, fields):
    """Apply FlexTime transformations for optional (*time.Time) fields."""
    for field in fields:
        # Getter body: return *o.Field → return o.Field.Time
        content = content.replace(
            f"return *o.{field}\n",
            f"return o.{field}.Time\n",
        )
        # Setter body: o.Field = &v → o.Field = flextime.PtrFlexTime(v)
        content = content.replace(
            f"o.{field} = &v\n",
            f"o.{field} = flextime.PtrFlexTime(v)\n",
        )
    return content


def transform_required_fields(content, fields):
    """Apply FlexTime transformations for required (non-pointer time.Time) fields."""
    for field in fields:
        escaped = re.escape(field)
        # Getter body: return o.Field → return o.Field.Time
        # Only match standalone return (not return &o.Field)
        content = re.sub(
            rf"(return) o\.{escaped}\n",
            rf"\1 o.{field}.Time\n",
            content,
        )
        # Setter body: o.Field = v → o.Field = flextime.FlexTime{Time: v}
        content = content.replace(
            f"o.{field} = v\n",
            f"o.{field} = flextime.FlexTime{{Time: v}}\n",
        )
        # Constructor body: this.Field = paramName → this.Field = flextime.FlexTime{Time: paramName}
        # The generated constructor parameter is camelCase (first letter lowered).
        param = field[0].lower() + field[1:]
        content = content.replace(
            f"this.{field} = {param}\n",
            f"this.{field} = flextime.FlexTime{{Time: {param}}}\n",
        )
    return content


def transform_model(filepath):
    """Apply all FlexTime transformations to a single model file.

    Returns True if the file was modified.
    """
    content = filepath.read_text()
    optional, required = find_time_fields(content)

    if not optional and not required:
        return False

    # Add import
    content = add_flextime_import(content)

    # Replace struct field types
    content = re.sub(
        r"(\t\w+\s+)\*time\.Time(\s+`json:)",
        r"\1*flextime.FlexTime\2",
        content,
    )
    content = re.sub(
        r"(\t\w+)\s+time\.Time(\s+`json:)",
        r"\1 flextime.FlexTime\2",
        content,
    )

    # Replace Ok getter return types: (*time.Time, bool) → (*flextime.FlexTime, bool)
    content = content.replace("(*time.Time, bool)", "(*flextime.FlexTime, bool)")

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
            print(f"  flextime: {model_file.relative_to(apis_dir)}")

    print(f">> flextime: transformed {count} model files")


if __name__ == "__main__":
    main()
