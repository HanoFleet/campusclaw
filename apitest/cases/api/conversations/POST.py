from client import MISSING, Case, agent_named, drop_conversations, one, request, rows, student, student_b, user_id


def run(base, env):
    c = Case("POST /api/conversations")
    token = student(base, env)
    uid = user_id(env, "student_a1")
    aid = agent_named(env, "student_a1", "数学答疑")
    other_agent = agent_named(env, "student_b1", "物理答疑")
    created = []
    try:
        status, out = request(base, "POST", "/api/conversations", token=token, body={"agent_id": aid})
        c.eq("选定本班已启用应用返回 201", status, 201)
        created.append(out.get("id"))
        row = rows(env, f"SELECT JSON_OBJECT('user_id', user_id, 'agent_id', agent_id, 'title', title) FROM conversations WHERE id={int(out.get('id', 0))}")
        c.eq("库里会话属于当前用户和选定应用", row, [{"user_id": uid, "agent_id": aid, "title": "新的提问"}])

        before = one(env, "SELECT COUNT(*) FROM conversations")
        missing = request(base, "POST", "/api/conversations", token=token, body={"agent_id": MISSING})
        cross = request(base, "POST", "/api/conversations", token=token, body={"agent_id": other_agent})
        c.eq("他班应用返回 404", cross[0], 404)
        c.eq("他班应用和不存在的应用响应完全一样", cross, missing)
        peer = request(base, "GET", f"/api/conversations/{int(out.get('id', 0))}", token=student_b(base, env))
        gone = request(base, "GET", f"/api/conversations/{MISSING}", token=student_b(base, env))
        c.eq("同伴读取别人的会话返回 404", peer[0], 404)
        c.eq("别人的会话和不存在的会话响应完全一样", peer, gone)
        c.eq("失败请求没有入库", one(env, "SELECT COUNT(*) FROM conversations"), before)
    finally:
        drop_conversations(env, created)
    return c.result()
