import json
import os
import pathlib
import secrets
import subprocess
import urllib.error
import urllib.request

REPO = pathlib.Path(__file__).resolve().parent.parent
MISSING = 99999999


def request(base, method, path, token="", body=None, timeout=60, raw=False):
    data = None if body is None else json.dumps(body).encode()
    headers = {}
    if data is not None:
        headers["Content-Type"] = "application/json"
    return send(base, method, path, token, data, headers, timeout, raw)


def upload(base, path, token, filename, content, field="file"):
    boundary = "apitest" + secrets.token_hex(8)
    data = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
        "Content-Type: application/octet-stream\r\n\r\n"
    ).encode() + content + f"\r\n--{boundary}--\r\n".encode()
    return send(base, "POST", path, token, data, {"Content-Type": "multipart/form-data; boundary=" + boundary}, 60, False)


def send(base, method, path, token, data, headers, timeout, raw):
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            content = resp.read()
            if raw:
                return resp.status, content
            text = content.decode()
            return resp.status, json.loads(text) if text else {}
    except urllib.error.HTTPError as err:
        text = err.read().decode()
        try:
            parsed = json.loads(text) if text else {}
        except json.JSONDecodeError:
            parsed = {"raw": text}
        return err.code, parsed


def stream(base, path, token, body, timeout=240):
    """发送一条提问，返回 (状态码, [(事件名, 数据), ...])。"""
    data = json.dumps(body).encode()
    headers = {"Content-Type": "application/json", "Authorization": "Bearer " + token}
    req = urllib.request.Request(base + path, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            text = resp.read().decode()
            status = resp.status
    except urllib.error.HTTPError as err:
        return err.code, [("http_error", json.loads(err.read().decode() or "{}"))]
    events = []
    for block in text.split("\n\n"):
        name, payload = "", ""
        for line in block.splitlines():
            if line.startswith("event: "):
                name = line[len("event: "):]
            elif line.startswith("data: "):
                payload += line[len("data: "):]
        if name:
            events.append((name, json.loads(payload) if payload else {}))
    return status, events


def login(base, username, password):
    status, body = request(base, "POST", "/api/login", body={"username": username, "password": password})
    if status != 200 or "token" not in body:
        raise RuntimeError(f"login {username} failed: {status}")
    return body["token"]


def teacher(base, env):
    return login(base, "teacher_a", env["SEED_TEACHER_PASSWORD"])


def teacher_b(base, env):
    return login(base, "teacher_b", env["SEED_TEACHER_PASSWORD"])


def student(base, env):
    return login(base, "student_a1", env["SEED_STUDENT_A_PASSWORD"])


def student_b(base, env):
    return login(base, "student_b1", env["SEED_STUDENT_B_PASSWORD"])


def load_env(path):
    env = {}
    if not os.path.exists(path):
        return env
    for line in open(path, encoding="utf-8"):
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        env[key] = value
    return env


def uniq(prefix):
    return f"{prefix}-{secrets.token_hex(3)}"


def q(value):
    """把字符串写成 SQL 字面量。"""
    return "'" + str(value).replace("\\", "\\\\").replace("'", "''") + "'"


def sql(env, query):
    """在 db 容器里执行 SQL，返回每行按制表符拆开的列。"""
    cmd = [
        "docker", "compose", "exec", "-T", "-e", "MYSQL_PWD=" + env["DB_PASSWORD"], "db",
        "mysql", "-u" + env["DB_USER"], "-N", "-B", "-r", "--default-character-set=utf8mb4", env["DB_NAME"], "-e", query,
    ]
    out = subprocess.run(cmd, cwd=REPO, capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError("sql failed: " + out.stderr.strip())
    return [line.split("\t") for line in out.stdout.splitlines()]


def rows(env, query):
    """query 只选一列 JSON_OBJECT(...)，每行解析成 dict。"""
    return [json.loads(r[0]) for r in sql(env, query)]


def one(env, query):
    result = sql(env, query)
    return result[0][0] if result else None


def user_id(env, username):
    return int(one(env, f"SELECT id FROM users WHERE username={q(username)}"))


def class_of(env, username):
    return int(one(env, f"SELECT class_id FROM users WHERE username={q(username)}"))


def agent_named(env, username, name):
    return int(one(env, f"SELECT id FROM tutor_agents WHERE class_id={class_of(env, username)} AND name={q(name)}"))


def drop_conversations(env, ids):
    ids = [int(i) for i in ids if i]
    if ids:
        joined = ",".join(map(str, ids))
        sql(env, f"DELETE FROM messages WHERE conversation_id IN ({joined}); DELETE FROM llm_exchanges WHERE conversation_id IN ({joined}); DELETE FROM conversations WHERE id IN ({joined})")


def drop_agents(env, ids):
    ids = [int(i) for i in ids if i]
    if ids:
        joined = ",".join(map(str, ids))
        convs = [r[0] for r in sql(env, f"SELECT id FROM conversations WHERE agent_id IN ({joined})")]
        drop_conversations(env, convs)
        sql(env, f"DELETE FROM tutor_agents WHERE id IN ({joined})")


def drop_skills(env, ids):
    ids = [int(i) for i in ids if i]
    if ids:
        joined = ",".join(map(str, ids))
        sql(env, f"DELETE FROM tutor_agent_skills WHERE skill_id IN ({joined}); DELETE FROM skills WHERE id IN ({joined})")


def drop_mcp(env, ids):
    ids = [int(i) for i in ids if i]
    if ids:
        joined = ",".join(map(str, ids))
        sql(env, f"DELETE FROM mcp_tools WHERE server_id IN ({joined}); DELETE FROM mcp_servers WHERE id IN ({joined})")


def mcp_base(env):
    return one(env, "SELECT base_url FROM mcp_servers WHERE name='campus-tools' ORDER BY id LIMIT 1") or "http://mcp:8090"


class Case:
    """一个接口脚本里的所有检查。每项检查都写进报告，任一项不通过整个脚本就算失败。"""

    def __init__(self, name):
        self.name = name
        self.notes = []
        self.ok = True

    def check(self, label, cond, info=""):
        mark = "✓" if cond else "✗"
        self.notes.append(f"{mark} {label}" + (f"（{info}）" if info and not cond else ""))
        if not cond:
            self.ok = False
        return cond

    def eq(self, label, got, want):
        return self.check(label, got == want, f"实际 {got!r}，应为 {want!r}")

    def result(self):
        return {"name": self.name, "ok": self.ok, "detail": "\n".join(self.notes)}
