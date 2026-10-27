import base64
import json

from client import Case, one, q, request, rows, user_id


def claims(token):
    part = token.split(".")[1]
    return json.loads(base64.urlsafe_b64decode(part + "=" * (-len(part) % 4)))


def run(base, env):
    c = Case("POST /api/login")
    uid = user_id(env, "teacher_a")
    status, body = request(base, "POST", "/api/login", body={"username": "teacher_a", "password": env["SEED_TEACHER_PASSWORD"]})
    c.eq("正确密码返回 200", status, 200)
    c.eq("令牌类型是 Bearer", body.get("token_type"), "Bearer")
    token = body.get("token", "")
    sid = claims(token).get("sub", "") if token.count(".") == 2 else ""
    stored = rows(env, f"SELECT JSON_OBJECT('id', id, 'expires', UNIX_TIMESTAMP(expires_at)) FROM sessions WHERE user_id={uid}")
    c.eq("同一账号库里只留最新一条会话", len(stored), 1)
    c.check("令牌里的会话编号就是库里那条", bool(stored) and stored[0]["id"] == sid, f"令牌 {sid}，库里 {stored}")
    c.check("令牌过期时间和库里一致", bool(stored) and abs(int(stored[0]["expires"]) - claims(token).get("exp", 0)) <= 2)
    c.eq("库里不存令牌原文", one(env, f"SELECT COUNT(*) FROM sessions WHERE id={q(token[:64])}"), "0")

    status, body = request(base, "POST", "/api/login", body={"username": "apitest-nobody", "password": "wrong"})
    c.eq("不存在的账号返回 401", status, 401)
    c.eq("错误码 invalid_credentials", body.get("error"), "invalid_credentials")

    status, body = request(base, "POST", "/api/login", body={"username": "teacher_a", "password": "x", "role": "teacher"})
    c.eq("多余字段返回 400", status, 400)
    c.eq("错误码 invalid_request", body.get("error"), "invalid_request")
    return c.result()
