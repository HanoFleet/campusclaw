from client import Case, class_of, request, rows, student


def chunk(env, hit):
    found = rows(env, f"""SELECT JSON_OBJECT('class_id', c.class_id, 'title', m.title, 'start', c.start_offset,
        'end', c.end_offset, 'text', c.chunk_text) FROM knowledge_chunks c JOIN materials m ON m.id=c.material_id
        WHERE c.material_id={int(hit['material_id'])} AND c.chunk_index={int(hit['chunk_index'])}""")
    return found[0] if found else {}


def run(base, env):
    c = Case("POST /api/search")
    token = student(base, env)
    mine = class_of(env, "student_a1")
    other = class_of(env, "student_b1")
    for mode in ["hybrid", "keyword", "vector"]:
        status, body = request(base, "POST", "/api/search", token=token, body={"query": "去括号", "mode": mode})
        c.eq(f"{mode} 检索返回 200", status, 200)
        c.eq(f"{mode} 返回的模式", body.get("mode"), mode)
        hits = body.get("hits", [])
        c.check(f"{mode} 有命中", len(hits) > 0)
        for hit in hits:
            row = chunk(env, hit)
            c.check(f"{mode} 命中「{hit['title']}」第 {hit['chunk_index']} 段在库里，属于本班", row.get("class_id") == mine)
            c.check(f"{mode} 命中的标题和起止位置与库里一致",
                    (row.get("title"), row.get("start"), row.get("end")) == (hit["title"], hit["start"], hit["end"]))
            c.check(f"{mode} 摘录出自这一段原文", hit["excerpt"].strip("…. \n")[:20] in row.get("text", ""))

    status, body = request(base, "POST", "/api/search", token=token, body={"query": "牛顿 外力", "class_id": other})
    hits = body.get("hits", [])
    c.check("请求体里伪造他班 class_id 也只检索本班", all(chunk(env, h).get("class_id") == mine for h in hits),
            str([h["title"] for h in hits]))
    status, body = request(base, "POST", "/api/search", token=token, body={"query": "  "})
    c.eq("空检索词返回 400", status, 400)
    c.eq("错误码 invalid_query", body.get("error"), "invalid_query")
    return c.result()
