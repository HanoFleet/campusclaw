from client import Case, login, one, request, user_id


def run(base, env):
    c = Case("POST /api/logout")
    uid = user_id(env, "student_a1")
    token = login(base, "student_a1", env["SEED_STUDENT_A_PASSWORD"])
    c.eq("登录后库里有会话", one(env, f"SELECT COUNT(*) FROM sessions WHERE user_id={uid}"), "1")
    status, body = request(base, "POST", "/api/logout", token=token)
    c.eq("退出返回 200", status, 200)
    c.eq("返回 status=ok", body.get("status"), "ok")
    c.eq("库里的会话已删除", one(env, f"SELECT COUNT(*) FROM sessions WHERE user_id={uid}"), "0")
    status, body = request(base, "GET", "/api/me", token=token)
    c.eq("旧令牌再用返回 401", status, 401)
    status, _ = request(base, "POST", "/api/logout")
    c.eq("不带令牌退出返回 401", status, 401)
    return c.result()
