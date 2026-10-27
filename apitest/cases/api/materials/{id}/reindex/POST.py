from client import MISSING, Case, class_of, one, request, rows, student, teacher, teacher_b


def run(base, env):
    c = Case("POST /api/materials/{id}/reindex")
    mid = int(one(env, f"SELECT id FROM materials WHERE class_id={class_of(env, 'teacher_a')} ORDER BY id LIMIT 1"))
    status, body = request(base, "POST", f"/api/materials/{mid}/reindex", token=student(base, env), body={"strategy": "auto"})
    c.eq("学生重建索引返回 403", status, 403)

    status, body = request(base, "POST", f"/api/materials/{mid}/reindex", token=teacher(base, env), body={"strategy": "auto"})
    c.eq("教师重建索引返回 200", status, 200)
    chunks = rows(env, f"""SELECT JSON_OBJECT('status', index_status, 'strategy', strategy, 'class_id', class_id)
        FROM knowledge_chunks WHERE material_id={mid}""")
    c.check("库里有切片", len(chunks) > 0)
    c.check("切片全部建好索引", all(ch["status"] == "ready" for ch in chunks), str({ch["status"] for ch in chunks}))
    c.check("切片的班级就是材料的班级", all(ch["class_id"] == class_of(env, "teacher_a") for ch in chunks))

    other = teacher_b(base, env)
    cross = request(base, "POST", f"/api/materials/{mid}/reindex", token=other, body={"strategy": "auto"})
    missing = request(base, "POST", f"/api/materials/{MISSING}/reindex", token=other, body={"strategy": "auto"})
    c.eq("他班教师重建返回 404", cross[0], 404)
    c.eq("他班材料和不存在的材料响应完全一样", cross, missing)
    return c.result()
