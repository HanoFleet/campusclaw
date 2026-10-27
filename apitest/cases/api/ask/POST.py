from client import Case, class_of, request, rows, student


def run(base, env):
    c = Case("POST /api/ask")
    token = student(base, env)
    mine = class_of(env, "student_a1")
    status, body = request(base, "POST", "/api/ask", token=token, body={"query": "水沸腾时会发生什么？"}, timeout=180)
    c.eq("提问返回 200", status, 200)
    c.check("有回答", len(body.get("answer", "").strip()) > 0)
    cites = body.get("citations", [])
    c.check("带出处", len(cites) > 0)
    for hit in cites:
        row = rows(env, f"""SELECT JSON_OBJECT('class_id', class_id) FROM knowledge_chunks
            WHERE material_id={int(hit['material_id'])} AND chunk_index={int(hit['chunk_index'])}""")
        c.check(f"出处「{hit['title']}」第 {hit['chunk_index']} 段在库里，属于本班", bool(row) and row[0]["class_id"] == mine)
    status, body = request(base, "POST", "/api/ask", token=token, body={"query": ""})
    c.eq("空提问返回 400", status, 400)
    return c.result()
