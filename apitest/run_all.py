#!/usr/bin/env python3
import importlib.util
import os
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from client import load_env

BASE = os.environ.get("CAMPUSCLAW_BASE", "http://127.0.0.1:8080")
ENV = load_env(ROOT.parent / ".env")


def load_case(path):
    spec = importlib.util.spec_from_file_location(path.stem + str(path.parent).replace("/", "_"), path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def main():
    results = []
    for path in sorted(ROOT.joinpath("cases").rglob("*.py")):
        mod = load_case(path)
        try:
            results.append(mod.run(BASE, ENV))
        except Exception as err:
            results.append({"name": str(path.relative_to(ROOT)), "ok": False, "detail": str(err)})
    report = ROOT / "report.html"
    rows = []
    for item in results:
        color = "#1b7f3a" if item["ok"] else "#8c1515"
        rows.append(f"<tr><td>{item['name']}</td><td style='color:{color}'>{'通过' if item['ok'] else '失败'}</td><td>{item['detail']}</td></tr>")
    report.write_text(
        "<!doctype html><meta charset='utf-8'><title>接口测试</title>"
        "<table border='1' cellpadding='6'><tr><th>接口</th><th>结果</th><th>说明</th></tr>"
        + "".join(rows)
        + "</table>",
        encoding="utf-8",
    )
    failed = [item for item in results if not item["ok"]]
    for item in results:
        print(("OK " if item["ok"] else "FAIL "), item["name"], item["detail"])
    print("report", report)
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
