#!/usr/bin/env python3
"""按本仓库规约请求已启动的 Compose，并把不含口令的结果写入 docs/verification.md。"""

from __future__ import annotations

import json
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
from http.cookiejar import CookieJar
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_env() -> dict[str, str]:
    env: dict[str, str] = {}
    for line in (ROOT / ".env").read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        env[key] = value
    return env


ENV = load_env()
BASE = f"http://127.0.0.1:{ENV.get('WEB_PORT', '8080')}"
LINES: list[str] = ["# 验收记录", "", f"目标：`{BASE}`", ""]


def note(text: str) -> None:
    print(text)
    LINES.append(text)


def fail(text: str) -> None:
    note(f"FAIL {text}")
    write_doc()
    raise SystemExit(1)


def write_doc() -> None:
    path = ROOT / "docs" / "verification.md"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(LINES) + "\n", encoding="utf-8")


def opener() -> urllib.request.OpenerDirector:
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(CookieJar()))


def call(op: urllib.request.OpenerDirector, method: str, path: str, body: bytes | None = None, headers: dict[str, str] | None = None):
    req = urllib.request.Request(BASE + path, data=body, headers=headers or {}, method=method)
    try:
        with op.open(req, timeout=30) as resp:
            raw = resp.read()
            return resp.status, raw, resp.headers
    except urllib.error.HTTPError as err:
        return err.code, err.read(), err.headers
    except urllib.error.URLError as err:
        raise ConnectionError(f"{method} {path} 无法连接: {err}") from err


def wait_health(op: urllib.request.OpenerDirector) -> None:
    for _ in range(90):
        try:
            code, raw, _ = call(op, "GET", "/health")
            if code == 200 and b"ok" in raw:
                return
        except (ConnectionError, TimeoutError):
            pass
        time.sleep(2)
    fail("等待 /health 超时")


def wait_login(username: str, password: str) -> urllib.request.OpenerDirector:
    last = ""
    for _ in range(90):
        op = opener()
        payload = json.dumps({"username": username, "password": password}).encode()
        try:
            code, raw, headers = call(op, "POST", "/api/login", payload, {"Content-Type": "application/json"})
        except (ConnectionError, TimeoutError) as err:
            last = str(err)
            time.sleep(2)
            continue
        if code == 200:
            cookie = headers.get("Set-Cookie") or ""
            if "HttpOnly" not in cookie or "SameSite=Lax" not in cookie:
                fail(f"Cookie 缺少 HttpOnly 或 SameSite=Lax: {cookie}")
            return op
        last = f"{code} {raw[:120]!r}"
        time.sleep(2)
    fail(f"登录 {username} 超时: {last}")


def login(username: str, password: str) -> urllib.request.OpenerDirector:
    op = opener()
    payload = json.dumps({"username": username, "password": password}).encode()
    code, raw, headers = call(op, "POST", "/api/login", payload, {"Content-Type": "application/json"})
    if code != 200:
        fail(f"登录 {username} 返回 {code} {raw[:200]!r}")
    cookie = headers.get("Set-Cookie") or ""
    if "HttpOnly" not in cookie or "SameSite=Lax" not in cookie:
        fail(f"Cookie 缺少 HttpOnly 或 SameSite=Lax: {cookie}")
    return op


def compose(*args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", "compose", *args],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=check,
    )


def mysql(sql: str) -> str:
    proc = subprocess.run(
        [
            "docker",
            "compose",
            "exec",
            "-T",
            "-e",
            f"MYSQL_PWD={ENV['DB_PASSWORD']}",
            "db",
            "mysql",
            "-N",
            "-u",
            ENV["DB_USER"],
            ENV["DB_NAME"],
            "-e",
            sql,
        ],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=False,
    )
    if proc.returncode != 0:
        fail(f"mysql 失败: {proc.stderr.strip()}")
    return proc.stdout.strip()


def main() -> None:
    anon = opener()
    note("## 启动与健康检查")
    wait_health(anon)
    code, raw, _ = call(anon, "GET", "/health")
    if code != 200 or b"ok" not in raw:
        fail(f"/health -> {code} {raw!r}")
    note(f"PASS GET /health {code} {raw.decode().strip()}")

    code, raw, _ = call(anon, "GET", "/api/materials")
    text = raw.decode()
    if code != 401 or "函数单调性" in text or "牛顿定律" in text:
        fail(f"未登录材料接口 -> {code} {text}")
    note(f"PASS 未登录 GET /api/materials {code} 且不含材料标题")

    code, raw, _ = call(anon, "GET", "/uploads/secret.md")
    if code == 200:
        fail("uploads 静态路径返回了 200")
    note(f"PASS GET /uploads/secret.md -> {code}")

    note("## 登录失败同形")
    missing = json.dumps({"username": "no-such-user", "password": "wrong-pass"}).encode()
    wrong = json.dumps({"username": "teacher_a", "password": "wrong-pass"}).encode()
    c1, b1, _ = call(anon, "POST", "/api/login", missing, {"Content-Type": "application/json"})
    c2, b2, _ = call(anon, "POST", "/api/login", wrong, {"Content-Type": "application/json"})
    if c1 != 401 or b1 != b2 or c1 != c2:
        fail(f"失败响应不一致 {c1} {b1!r} vs {c2} {b2!r}")
    note(f"PASS 不存在账号与错误口令同为 {c1} {b1.decode().strip()}")

    note("## 主路径")
    teacher = login("teacher_a", ENV["SEED_TEACHER_PASSWORD"])
    code, raw, _ = call(teacher, "GET", "/api/me")
    me = json.loads(raw)
    if code != 200 or me.get("role") != "teacher" or me.get("class_name") != "A":
        fail(f"/api/me {code} {raw!r}")
    note(f"PASS teacher_a role={me['role']} class={me['class_name']}")

    code, raw, _ = call(teacher, "GET", "/api/materials?q=" + urllib.parse.quote("牛顿定律") + "&class_id=2")
    body = json.loads(raw)
    titles = [item["title"] for item in body["materials"]]
    if code != 200 or any("牛顿" in title for title in titles):
        fail(f"A 班搜索泄漏: {titles}")
    note("PASS A 班搜索「牛顿定律」且带 class_id 参数时没有 B 班标题")

    stamp = str(int(time.time()))
    upload_title = f"验收-上传-{stamp}.md"
    content = f"# 验收\n\n这是教师上传的正文 {stamp}。\n\n<script>alert(1)</script>\n"
    boundary = f"----campusclaw{stamp}"
    form = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="class_id"\r\n\r\n999\r\n'
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="{upload_title}"\r\n'
        f"Content-Type: text/markdown\r\n\r\n{content}\r\n"
        f"--{boundary}--\r\n"
    ).encode()
    code, raw, _ = call(
        teacher,
        "POST",
        "/api/materials",
        form,
        {"Content-Type": f"multipart/form-data; boundary={boundary}"},
    )
    if code != 201:
        fail(f"上传 -> {code} {raw!r}")
    created = json.loads(raw)
    material_id = created["id"]
    note(f"PASS 教师上传 201 id={material_id}")

    code, raw, _ = call(teacher, "GET", f"/api/materials/{material_id}")
    detail = json.loads(raw)
    if code != 200 or detail.get("body") != content or detail.get("class_id") != me["class_id"]:
        fail(f"详情不符合预期 {code} {raw[:300]!r}")
    if "storage_name" in detail or "/uploads/" in json.dumps(detail):
        fail("详情含存储路径")
    note("PASS 详情正文与文件一致，班级仍是会话班级，且没有存储路径")

    code, raw, _ = call(teacher, "GET", "/api/materials")
    titles = [item["title"] for item in json.loads(raw)["materials"]]
    if upload_title not in titles or "A班-函数单调性讲义" not in titles:
        fail(f"列表缺少材料: {titles}")
    note("PASS 本班列表能看到新上传和预置讲义")

    student_a = login("student_a1", ENV["SEED_STUDENT_A_PASSWORD"])
    code, raw, _ = call(student_a, "GET", f"/api/materials/{material_id}/file")
    if code != 200 or stamp.encode() not in raw:
        fail(f"学生下载失败 {code}")
    note("PASS 同班学生可以下载")

    before = mysql("SELECT COUNT(*) FROM materials")
    code, raw, _ = call(
        student_a,
        "POST",
        "/api/materials",
        form,
        {"Content-Type": f"multipart/form-data; boundary={boundary}"},
    )
    after = mysql("SELECT COUNT(*) FROM materials")
    if code != 403 or before != after:
        fail(f"学生上传应为 403 且行数不变，实际 {code} {before}->{after} {raw!r}")
    note(f"PASS 学生上传 403，材料行数保持 {after}")

    student_b = login("student_b1", ENV["SEED_STUDENT_B_PASSWORD"])
    code_b, raw_b, _ = call(student_b, "GET", f"/api/materials/{material_id}")
    code_missing, raw_missing, _ = call(student_b, "GET", "/api/materials/999999")
    if code_b != 404 or code_missing != 404 or raw_b != raw_missing:
        fail(f"跨班 404 与不存在 id 不同 {code_b} {raw_b!r} vs {code_missing} {raw_missing!r}")
    if "验收".encode() in raw_b or b"alert" in raw_b:
        fail("跨班响应含正文")
    note("PASS 跨班详情与不存在 id 的 404 响应体相同")

    code, raw, _ = call(student_b, "GET", "/api/materials?q=" + urllib.parse.quote("函数单调性"))
    titles = [item["title"] for item in json.loads(raw)["materials"]]
    if any("函数单调性" in title or title == upload_title for title in titles):
        fail(f"B 班看到了 A 班材料: {titles}")
    code, raw, _ = call(student_b, "GET", "/api/materials")
    b_titles = [item["title"] for item in json.loads(raw)["materials"]]
    if "B班-牛顿定律笔记" not in b_titles:
        fail("B 班看不到自己的预置笔记")
    note("PASS B 班看不到 A 班材料，能看到自己的预置笔记")

    exe_boundary = f"----exe{stamp}"
    exe_form = (
        f"--{exe_boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="notes.md.exe"\r\n'
        f"Content-Type: application/octet-stream\r\n\r\nMZ\r\n"
        f"--{exe_boundary}--\r\n"
    ).encode()
    before = mysql("SELECT COUNT(*) FROM materials")
    code, raw, _ = call(
        teacher,
        "POST",
        "/api/materials",
        exe_form,
        {"Content-Type": f"multipart/form-data; boundary={exe_boundary}"},
    )
    after = mysql("SELECT COUNT(*) FROM materials")
    if code != 400 or before != after:
        fail(f".exe 应为 400 且无残留，实际 {code} {before}->{after}")
    note("PASS notes.md.exe 返回 400，材料行数不变")

    note("## 种子与密钥")
    hashes = mysql("SELECT username, password_hash FROM users ORDER BY username")
    for line in hashes.splitlines():
        username, pw_hash = line.split("\t", 1)
        plain = {
            "teacher_a": ENV["SEED_TEACHER_PASSWORD"],
            "student_a1": ENV["SEED_STUDENT_A_PASSWORD"],
            "student_b1": ENV["SEED_STUDENT_B_PASSWORD"],
        }[username]
        if not pw_hash.startswith("$2") or pw_hash == plain:
            fail(f"{username} 口令不是 bcrypt")
    note("PASS 三个预置账号的 password_hash 都是 bcrypt，且不等于明文")

    tables = set(mysql("SHOW TABLES").split())
    needed = {"classes", "users", "handouts", "assignments", "assistants", "skills", "materials", "knowledge_entries", "sessions"}
    if not needed <= tables:
        fail(f"缺表: {needed - tables}")
    nullable = mysql(
        "SELECT IS_NULLABLE FROM information_schema.COLUMNS "
        "WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='materials' AND COLUMN_NAME='class_id'"
    )
    if nullable != "NO":
        fail(f"materials.class_id 可空: {nullable}")
    note("PASS 九张表存在，materials.class_id 为 NOT NULL")

    root_env = subprocess.run(
        ["docker", "compose", "exec", "-T", "api", "printenv", "MYSQL_ROOT_PASSWORD"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=False,
    )
    if root_env.stdout.strip():
        fail("api 容器读到了 MYSQL_ROOT_PASSWORD")
    note("PASS api 容器没有 MYSQL_ROOT_PASSWORD")

    note("## 登出")
    logout_op = login("student_a1", ENV["SEED_STUDENT_A_PASSWORD"])
    code, _, _ = call(logout_op, "POST", "/api/logout")
    if code != 200:
        fail(f"登出 {code}")
    code, _, _ = call(logout_op, "GET", "/api/me")
    if code != 401:
        fail(f"登出后旧 Cookie 仍可用 {code}")
    note("PASS 登出后旧 Cookie 得到 401")

    note("## 数据库停止时 /health 与业务接口分离")
    compose("stop", "db")
    try:
        time.sleep(2)
        code, raw, _ = call(anon, "GET", "/health")
        if code != 200:
            fail(f"停库后 /health 不是 200: {code}")
        code, raw, _ = call(anon, "GET", "/api/materials")
        if code != 503:
            fail(f"停库后材料接口不是 503: {code} {raw!r}")
        note("PASS 停库后 /health 200，/api/materials 503")
    finally:
        compose("start", "db")
    wait_login("teacher_a", ENV["SEED_TEACHER_PASSWORD"])
    note("PASS 数据库恢复后可以重新登录")

    note("## 限流与口令错误同形")
    locked_body = None
    for _ in range(int(ENV["LOGIN_FAIL_THRESHOLD"])):
        _, locked_body, _ = call(anon, "POST", "/api/login", wrong, {"Content-Type": "application/json"})
    good = json.dumps({"username": "teacher_a", "password": ENV["SEED_TEACHER_PASSWORD"]}).encode()
    code, raw, _ = call(anon, "POST", "/api/login", good, {"Content-Type": "application/json"})
    if code != 401 or raw != locked_body:
        fail(f"锁定期正确口令应与错误口令同形，实际 {code} {raw!r}")
    note("PASS 锁定期内正确口令仍返回与错误口令相同的 401")
    compose("restart", "api")
    wait_health(anon)
    note("PASS 重启 api 以清掉内存中的限流计数")

    note("## 重建容器后数据仍在")
    users_before = mysql("SELECT COUNT(*) FROM users")
    compose("down")
    compose("up", "-d")
    teacher = wait_login("teacher_a", ENV["SEED_TEACHER_PASSWORD"])
    code, raw, _ = call(teacher, "GET", "/api/materials")
    titles = [item["title"] for item in json.loads(raw)["materials"]]
    users_after = mysql("SELECT COUNT(*) FROM users")
    if upload_title not in titles or users_before != users_after:
        fail(f"重启后数据丢失 titles={titles} users {users_before}->{users_after}")
    note(f"PASS down/up 后仍能看到 {upload_title}，用户数仍为 {users_after}")

    frontend = (ROOT / "frontend" / "src" / "App.tsx").read_text()
    if "rehype-raw" in frontend or "dangerouslySetInnerHTML" in frontend:
        fail("前端打开了原始 HTML 渲染")
    if "ReactMarkdown" not in frontend:
        fail("前端没有用 Markdown 渲染")
    note("PASS 前端使用 ReactMarkdown，没有 rehype-raw 或 dangerouslySetInnerHTML")

    note("")
    note("全部断言通过。")
    write_doc()


if __name__ == "__main__":
    try:
        main()
    except ConnectionError as err:
        fail(str(err))
