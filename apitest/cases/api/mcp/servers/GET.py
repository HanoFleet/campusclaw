from client import MISSING, Case, class_of, one, request, rows, student, teacher, teacher_b


def run(base, env):
    c = Case("GET /api/mcp/servers")
    token = teacher(base, env)
    cls = class_of(env, "teacher_a")
    status, body = request(base, "GET", "/api/mcp/servers", token=token)
    c.eq("教师返回 200", status, 200)
    servers = body.get("servers") or []
    want = rows(env, f"SELECT JSON_OBJECT('id', id, 'name', name, 'connected', connected=1, 'has_token', auth_token<>'') FROM mcp_servers WHERE class_id={cls} ORDER BY id")
    got = [{k: s[k] for k in ("id", "name", "connected", "has_token")} for s in servers]
    c.eq("列表与库里本班服务器一致", got, [dict(r, connected=bool(r["connected"]), has_token=bool(r["has_token"])) for r in want])
    raw = str(body)
    c.check("整份返回里搜不到令牌字段", "auth_token" not in raw and "MCP_AUTH_TOKEN" not in raw)
    token_val = env.get("MCP_AUTH_TOKEN", "")
    c.check("整份返回里搜不到令牌字符串", not token_val or token_val not in raw)

    other = teacher_b(base, env)
    mine_id = int(one(env, f"SELECT id FROM mcp_servers WHERE class_id={cls} ORDER BY id LIMIT 1") or 0)
    c.check("本班至少登记了一台服务器", mine_id > 0)
    cross = request(base, "GET", f"/api/mcp/servers/{mine_id}/tools", token=other)
    missing = request(base, "GET", f"/api/mcp/servers/{MISSING}/tools", token=other)
    c.eq("他班教师按编号访问返回 404", cross[0], 404)
    c.eq("他班服务器和不存在的服务器响应完全一样", cross, missing)

    status, body = request(base, "GET", "/api/mcp/servers", token=student(base, env))
    c.eq("学生也可以列出本班服务器", status, 200)
    c.check("学生也看不到令牌", env.get("MCP_AUTH_TOKEN", "") not in str(body))
    return c.result()
