from client import Case, class_of, request, rows, student, teacher, teacher_b


def run(base, env):
    c = Case("GET /api/mcp/audits")
    token = teacher(base, env)
    cls = class_of(env, "teacher_a")
    status, body = request(base, "GET", "/api/mcp/audits?limit=20", token=token)
    c.eq("教师查看返回 200", status, 200)
    c.eq("limit 回显", body.get("limit"), 20)
    audits = body.get("audits") or []
    want = rows(env, f"""SELECT JSON_OBJECT('id', id, 'ref', ref_text, 'status', status, 'round', round_no)
        FROM mcp_calls WHERE class_id={cls} ORDER BY id DESC LIMIT 20""")
    got = [{k: a[k] for k in ("id", "ref", "status", "round")} for a in audits]
    c.eq("记录与库里本班调用一致", got, want)
    for item in audits:
        c.check("一条记录能回答调了什么", bool(item.get("ref")) and "@" in str(item.get("ref")))
        c.check("一条记录有状态", item.get("status") in ("ok", "denied", "error"), str(item.get("status")))
        c.check("一条记录有触发者", "username" in item)
        c.check("一条记录有对话和轮次", "conversation_id" in item and "round" in item)
        c.check("一条记录有时间", bool(item.get("created_at")))

    other = request(base, "GET", "/api/mcp/audits", token=teacher_b(base, env))
    c.eq("他班教师也能打开自己的记录", other[0], 200)
    other_ids = {a["id"] for a in (other[1].get("audits") or [])}
    mine_ids = {a["id"] for a in audits}
    c.check("他班教师看不到本班调用", other_ids.isdisjoint(mine_ids), str(other_ids & mine_ids))
    status, body = request(base, "GET", "/api/mcp/audits", token=student(base, env))
    c.eq("学生查看返回 403", status, 403)
    status, body = request(base, "GET", "/api/mcp/audits?limit=0", token=token)
    c.eq("limit 越界返回 400", status, 400)
    return c.result()
