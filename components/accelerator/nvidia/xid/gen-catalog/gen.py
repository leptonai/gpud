#!/usr/bin/env python3
"""Generate Go catalog data for NVIDIA XID catalog.

Usage with ``uv`` (https://github.com/astral-sh/uv):

    uv run python components/accelerator/nvidia/xid/gen-catalog/gen.py \
        Xid-Catalog.xlsx \
        components/accelerator/nvidia/xid/catalog_generated.go

``uv run`` ensures a consistent Python environment without polluting the
system interpreter. The script requires no third-party dependencies, so the
default project virtual environment created by ``uv`` is sufficient.

Parses the provided XID catalog Excel file (sheet1 and sheet2) and writes Go
data structures to the specified output path.
"""
import datetime
import sys
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import List, Dict, Any

NS = {"main": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}


def load_shared_strings(zf: zipfile.ZipFile) -> List[str]:
    shared_strings: List[str] = []
    if "xl/sharedStrings.xml" not in zf.namelist():
        return shared_strings
    root = ET.fromstring(zf.read("xl/sharedStrings.xml"))
    for si in root.findall("main:si", NS):
        text_fragments: List[str] = []
        for node in si.findall(".//main:t", NS):
            if node.text:
                text_fragments.append(node.text)
        shared_strings.append("".join(text_fragments))
    return shared_strings


def read_sheet(zf: zipfile.ZipFile, sheet_name: str, shared_strings: List[str]) -> List[List[str]]:
    sheet = ET.fromstring(zf.read(sheet_name))
    rows: List[List[str]] = []
    for row in sheet.findall("main:sheetData/main:row", NS):
        values: List[str] = []
        for c in row.findall("main:c", NS):
            cell_type = c.get("t")
            v = c.find("main:v", NS)
            if v is None:
                values.append("")
                continue
            if cell_type == "s":
                idx = int(v.text)
                values.append(shared_strings[idx])
            else:
                values.append(v.text or "")
        if values:
            rows.append(values)
    return rows


def escape_go_string(value: str) -> str:
    escaped = (
        value.replace("\\", "\\\\")
        .replace("\"", "\\\"")
        .replace("\n", "\\n")
    )
    return f'"{escaped}"'


def build_catalog_entries(rows: List[List[str]]) -> List[Dict[str, Any]]:
    header = rows[0]
    idx_type = header.index("Type \n(XID)")
    idx_code = header.index("Code")
    idx_mnemonic = header.index("Mnemonic")
    idx_desc = header.index("Description")
    idx_imm = header.index("Resolution Bucket \n(Immediate Action)")
    idx_inv = header.index("Resolution Bucket \n(Investigatory Action)")

    entries: List[Dict[str, Any]] = []
    for row in rows[1:]:
        # pad to header length
        if len(row) < len(header):
            row = row + [""] * (len(header) - len(row))
        if row[idx_type] != "XID":
            continue
        code_str = row[idx_code]
        if not code_str or not code_str.isdigit():
            continue
        entries.append(
            {
                "code": int(code_str),
                "mnemonic": row[idx_mnemonic].strip(),
                "description": row[idx_desc].strip(),
                "immediate": row[idx_imm].strip(),
                "investigatory": row[idx_inv].strip(),
            }
        )
    entries.sort(key=lambda item: item["code"])
    return entries


def build_nvlink_rules(rows: List[List[str]]) -> List[Dict[str, Any]]:
    header = rows[0]
    idx_xid = header.index("Xid")
    idx_subcode = header.index(
        "Subcode V1(<R575)/V2(>=R575)\nV1(<R575): IntrInfo[9:5]\nV2(>=R575):IntrInfo[6:0]"
    )
    idx_intrinfo_v1 = header.index(
        "(V1(<R575)) IntrInfo decode for Data Center Recovery Action \nIntrInfo (binary; \"-\" user decode)"
    )
    idx_intrinfo_v2 = header.index(
        "(V2(>=R575)) IntrInfo decode for Data Center Recovery Action\nIntrInfo (binary; \"-\" user decode)"
    )
    idx_error_status = header.index("Error Status (hex)")
    idx_resolution = header.index("Resolution Bucket \n(Data Center Recovery Action)")
    idx_action2 = header.index("Action 2") if "Action 2" in header else None
    idx_investigatory = header.index("Resolution Bucket \n(Investigatory Action)")
    idx_severity = header.index("Severity (for items with '*' please see Customer User Guide tab)")
    idx_hw_sw = header.index("HW/SW") if "HW/SW" in header else None
    idx_local_remote = header.index("Local/Remote (for items with '*' please see Customer User Guide tab)") if "Local/Remote (for items with '*' please see Customer User Guide tab)" in header else None

    rules: List[Dict[str, Any]] = []
    for row in rows[1:]:
        if len(row) < len(header):
            row = row + [""] * (len(header) - len(row))
        xid_str = row[idx_xid].strip()
        subcode = row[idx_subcode].strip()
        if not xid_str or not xid_str.isdigit() or not subcode:
            continue
        error_status_str = row[idx_error_status].strip() or "0x0"
        try:
            error_status = int(error_status_str, 16)
        except ValueError:
            # Some rows have placeholders like 'N/A'
            continue
        rule = {
            "xid": int(xid_str),
            "unit": subcode,
            "intrinfo_v1": row[idx_intrinfo_v1].strip(),
            "intrinfo_v2": row[idx_intrinfo_v2].strip(),
            "error_status": error_status,
            "resolution": row[idx_resolution].strip(),
            "investigatory": row[idx_investigatory].strip(),
            "severity": row[idx_severity].strip(),
        }
        if idx_action2 is not None:
            rule["action2"] = row[idx_action2].strip()
        if idx_hw_sw is not None:
            rule["hw_sw"] = row[idx_hw_sw].strip()
        if idx_local_remote is not None:
            rule["local_remote"] = row[idx_local_remote].strip()
        rules.append(rule)
    rules.sort(key=lambda r: (r["xid"], r["unit"], r["error_status"]))
    return rules


def write_go(entries: List[Dict[str, Any]], rules: List[Dict[str, Any]]) -> str:
    lines: List[str] = []
    timestamp = datetime.datetime.utcnow().isoformat(timespec="seconds") + "Z"
    lines.append("// Code generated by gen-catalog/gen.py; DO NOT EDIT.")
    lines.append(f"// Generated at {timestamp}. Source: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html")
    lines.append("")
    lines.append("package xid")
    lines.append("")
    lines.append("// catalogEntries mirrors NVIDIA's XID catalog (sheet: \"Catalog\").")
    lines.append("var catalogEntries = []catalogEntry{")
    for entry in entries:
        lines.append(
            "\t{"
            f"Code: {entry['code']}, "
            f"Mnemonic: {escape_go_string(entry['mnemonic'])}, "
            f"Description: {escape_go_string(entry['description'])}, "
            f"ImmediateResolution: {escape_go_string(entry['immediate'])}, "
            f"InvestigatoryResolution: {escape_go_string(entry['investigatory'])},"
            "},"
        )
    lines.append("}")
    lines.append("")
    lines.append("// nvlinkRules captures the NVLink5-specific decode table (sheet: \"XID 144-150 Decode\").")
    lines.append("var nvlinkRules = []nvlinkRule{")
    for rule in rules:
        line = (
            "\t{"
            f"Xid: {rule['xid']}, "
            f"Unit: {escape_go_string(rule['unit'])}, "
            f"IntrinfoPatternV1: {escape_go_string(rule['intrinfo_v1'])}, "
            f"IntrinfoPatternV2: {escape_go_string(rule['intrinfo_v2'])}, "
            f"ErrorStatus: 0x{rule['error_status']:08x}, "
            f"Resolution: {escape_go_string(rule['resolution'])}, "
            f"Investigatory: {escape_go_string(rule['investigatory'])}, "
            f"Severity: {escape_go_string(rule['severity'])}"
        )
        if "action2" in rule and rule["action2"]:
            line += f", Action2: {escape_go_string(rule['action2'])}"
        if "hw_sw" in rule and rule["hw_sw"]:
            line += f", HwSw: {escape_go_string(rule['hw_sw'])}"
        if "local_remote" in rule and rule["local_remote"]:
            line += f", LocalRemote: {escape_go_string(rule['local_remote'])}"
        line += "},"
        lines.append(line)
    lines.append("}")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    if len(sys.argv) != 3:
        sys.stderr.write(
            "Usage: gen.py <path-to-Xid-Catalog.xlsx> <path-to-output-go-file>\n"
        )
        return 1

    xlsx_path = Path(sys.argv[1]).expanduser().resolve()
    output_path = Path(sys.argv[2]).expanduser().resolve()

    if not xlsx_path.is_file():
        sys.stderr.write(f"error: catalog file not found: {xlsx_path}\n")
        return 1

    with zipfile.ZipFile(xlsx_path) as zf:
        shared_strings = load_shared_strings(zf)
        sheet1 = read_sheet(zf, "xl/worksheets/sheet1.xml", shared_strings)
        sheet2 = read_sheet(zf, "xl/worksheets/sheet2.xml", shared_strings)

    entries = build_catalog_entries(sheet1)
    rules = build_nvlink_rules(sheet2)

    go_code = write_go(entries, rules)
    output_path.write_text(go_code, encoding="utf-8")
    print(f"Generated catalog data to {output_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
