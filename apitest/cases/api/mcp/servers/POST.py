from client import Case, drop_mcp, mcp_base, one, request, rows, student, teacher, uniq


def run(base, env):
    c = Case("POST /api/mcp/servers")
    token = teacher(base, env)
    name = uniq("tools")[:20]
    created = []
    try:
        status, out = request(base, "POST", "/api/mcp/servers", token=token, body={
            "name": name, "base_url": mcp_base(env), "token": env["MCP_AUTH_TOKEN"],
        })
        c.eq("登记返回 201", status, 201)
        created.append(out.get("id"))
        c.eq("响应只回 has_token，不回令牌", ("has_token" in out) and ("token" not in out) and ("auth_token" not in out), True)
        c.eq("响应里没有令牌字符串", env["MCP_AUTH_TOKEN"] in str(out), False)
        row = rows(env, f"SELECT JSON_OBJECT('name', name, 'connected', connected, 'token', auth_token) FROM mcp_servers WHERE id={int(out.get('id', 0))}")
        c.eq("令牌只存在服务端库里", row[0]["token"] if row else "", env["MCP_AUTH_TOKEN"])
        c.eq("刚登记时未连接", row[0]["connected"] if row else 1, 0)

        before = one(env, "SELECT COUNT(*) FROM mcp_servers")
        status, out = request(base, "POST", "/api/mcp/servers", token=token, body={"name": name, "base_url": mcp_base(env)})
        c.eq("同班重名返回 400", (status, out.get("error")), (400, "duplicate_name"))
        status, out = request(base, "POST", "/api/mcp/servers", token=student(base, env), body={"name": uniq("stu"), "base_url": mcp_base(env)})
        c.eq("学生登记返回 403", status, 403)
        c.eq("被拒的请求没有入库", one(env, "SELECT COUNT(*) FROM mcp_servers"), before)
    finally:
        drop_mcp(env, created)
    return c.result()
