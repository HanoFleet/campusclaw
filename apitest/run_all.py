#!/usr/bin/env python3
"""运行 cases 下全部接口脚本，生成 report.html。

目录按接口路径组织，文件名是请求方法，例如 cases/api/skills/{id}/PUT.py 测 PUT /api/skills/{id}。
每个脚本既检查接口返回，也用 docker compose exec db mysql 查库，核对返回和库里一致。
可以带一个参数只跑路径里含这段文字的脚本，例如 python3 apitest/run_all.py mcp。
"""
import html
import importlib.util
import os
import pathlib
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from client import load_env

BASE = os.environ.get("CAMPUSCLAW_BASE", "http://127.0.0.1:8080")
ENV = load_env(ROOT.parent / ".env")


def load_case(path):
    spec = importlib.util.spec_from_file_location("case_" + str(abs(hash(str(path)))), path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def main():
    only = sys.argv[1] if len(sys.argv) > 1 else ""
    results = []
    for path in sorted(ROOT.joinpath("cases").rglob("*.py")):
        rel = str(path.relative_to(ROOT / "cases"))
        if only and only not in rel:
            continue
        started = time.time()
        try:
            item = load_case(path).run(BASE, ENV)
        except Exception as err:
            item = {"name": rel, "ok": False, "detail": f"{type(err).__name__}: {err}"}
        item["file"] = rel
        item["seconds"] = time.time() - started
        results.append(item)
        print(("OK  " if item["ok"] else "FAIL"), f"{item['seconds']:5.1f}s", item["name"])
        if not item["ok"]:
            print("     " + item["detail"].replace("\n", "\n     "))

    passed = sum(1 for item in results if item["ok"])
    rows = []
    for item in results:
        color = "#1b7f3a" if item["ok"] else "#8c1515"
        rows.append(
            f"<tr><td>{html.escape(item['name'])}<br><small>{html.escape(item['file'])}</small></td>"
            f"<td style='color:{color}'>{'通过' if item['ok'] else '失败'}</td>"
            f"<td>{item['seconds']:.1f}s</td>"
            f"<td style='white-space:pre-wrap'>{html.escape(item['detail'])}</td></tr>"
        )
    report = ROOT / "report.html"
    report.write_text(
        "<!doctype html><meta charset='utf-8'><title>接口测试报告</title>"
        "<style>body{font-family:sans-serif;margin:24px}td,th{vertical-align:top;text-align:left}</style>"
        f"<h1>接口测试报告</h1><p>{BASE}，共 {len(results)} 个脚本，通过 {passed} 个，失败 {len(results) - passed} 个。</p>"
        "<table border='1' cellpadding='6' cellspacing='0'><tr><th>接口</th><th>结果</th><th>用时</th><th>检查项</th></tr>"
        + "".join(rows)
        + "</table>",
        encoding="utf-8",
    )
    print(f"{passed}/{len(results)} passed, report {report}")
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
