from client import MISSING, Case, agent_named, drop_conversations, one, request, rows, stream, student, student_b


def run(base, env):
    c = Case("POST /api/conversations/{id}/messages")
    token = student(base, env)
    aid = agent_named(env, "student_a1", "数学答疑")
    created = []
    try:
        status, conv = request(base, "POST", "/api/conversations", token=token, body={"agent_id": aid})
        c.eq("先新建会话", status, 201)
        cid = int(conv.get("id", 0))
        created.append(cid)

        status, events = stream(base, f"/api/conversations/{cid}/messages", token, {"query": "去括号时要检查什么"})
        names = [e[0] for e in events]
        c.eq("流式接口返回 200", status, 200)
        c.check("先有 meta 再有 delta", "meta" in names and "delta" in names and names.index("meta") < names.index("delta"), str(names))
        c.check("有 done 收尾", names[-1] == "done" if names else False, str(names))
        meta = next((d for n, d in events if n == "meta"), {})
        answer = "".join(d.get("text", "") for n, d in events if n == "delta")
        c.eq("命中解题引导技能", (meta.get("skill") or {}).get("name"), "数学解题引导")
        cites = meta.get("citations") or []
        c.check("引用列表不空", len(cites) > 0)
        cited = "[1]" in answer or any((cite.get("title") or "") in answer for cite in cites)
        c.check("回答引用了这次检索到的资料", cited, answer[:120])
        c.check("回答不写出最终得数", "10" not in answer and "x=" not in answer.lower(), answer[:80])
        stored = rows(env, f"SELECT JSON_OBJECT('role', role, 'skill', IFNULL(skill_name,''), 'content', content) FROM messages WHERE conversation_id={cid} ORDER BY id")
        c.eq("库里先有用户提问", stored[0]["role"] if stored else "", "user")
        c.eq("库里用户消息就是这次提问", stored[0]["content"] if stored else "", "去括号时要检查什么")
        c.eq("库里助手消息与流式正文一致", stored[-1]["content"] if stored else "", answer)
        c.eq("库里记下了选中的技能", stored[-1]["skill"] if stored else "", "数学解题引导")

        peer = stream(base, f"/api/conversations/{cid}/messages", student_b(base, env), {"query": "再问一句"})
        gone = stream(base, f"/api/conversations/{MISSING}/messages", student_b(base, env), {"query": "再问一句"})
        c.eq("同伴向别人的会话提问返回 404", peer[0], 404)
        c.eq("别人的会话和不存在的会话提问响应完全一样", peer, gone)
        c.eq("提问少于 2 个字返回 400", request(base, "POST", f"/api/conversations/{cid}/messages", token=token, body={"query": "啊"})[0], 400)
        c.eq("失败提问没有多写入库", one(env, f"SELECT COUNT(*) FROM messages WHERE conversation_id={cid}"), str(len(stored)))
    finally:
        drop_conversations(env, created)
    return c.result()
