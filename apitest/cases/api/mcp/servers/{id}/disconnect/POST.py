from client import Case, agent_named, class_of, drop_conversations, one, request, stream, student, teacher


def run(base, env):
    c = Case("POST /api/mcp/servers/{id}/disconnect")
    token = teacher(base, env)
    stu = student(base, env)
    cls = class_of(env, "teacher_a")
    sid = int(one(env, f"SELECT id FROM mcp_servers WHERE class_id={cls} AND name='campus-tools' ORDER BY id LIMIT 1"))
    created = []
    try:
        status, _ = request(base, "POST", f"/api/mcp/servers/{sid}/connect", token=token)
        c.eq("先连接成功", status, 200)
        aid = agent_named(env, "student_a1", "数学答疑")
        status, conv = request(base, "POST", "/api/conversations", token=stu, body={"agent_id": aid})
        created.append(conv.get("id"))
        cid = int(conv.get("id", 0))
        status, events = stream(base, f"/api/conversations/{cid}/messages", stu, {"query": "3 米是多少厘米"})
        tools = [d for n, d in events if n == "tool"]
        c.check("连接时提问会真正调用工具", any(t.get("status") == "ok" and "unit_convert" in str(t.get("ref")) for t in tools), str(tools))

        status, _ = request(base, "POST", f"/api/mcp/servers/{sid}/disconnect", token=token)
        c.eq("断开返回 200", status, 200)
        c.eq("库里标记为未连接", one(env, f"SELECT connected FROM mcp_servers WHERE id={sid}"), "0")
        c.eq("工具清单已清空", one(env, f"SELECT COUNT(*) FROM mcp_tools WHERE server_id={sid}"), "0")

        status, conv2 = request(base, "POST", "/api/conversations", token=stu, body={"agent_id": aid})
        created.append(conv2.get("id"))
        before = int(one(env, f"SELECT COUNT(*) FROM mcp_calls WHERE class_id={cls} AND status='denied'"))
        status, events = stream(base, f"/api/conversations/{int(conv2.get('id', 0))}/messages", stu, {"query": "3 米是多少厘米"})
        tools = [d for n, d in events if n == "tool"]
        c.check("断开后调用被记为被拒", any(t.get("status") == "denied" for t in tools), str(tools))
        c.check("记录里多出一条拒绝", int(one(env, f"SELECT COUNT(*) FROM mcp_calls WHERE class_id={cls} AND status='denied'")) > before)

        status, _ = request(base, "POST", f"/api/mcp/servers/{sid}/connect", token=token)
        c.eq("测完重新连接", status, 200)
        status, out = request(base, "POST", f"/api/mcp/servers/{sid}/disconnect", token=stu)
        c.eq("学生断开返回 403", status, 403)
    finally:
        drop_conversations(env, created)
        request(base, "POST", f"/api/mcp/servers/{sid}/connect", token=token)
    return c.result()
