from client import MISSING, Case, class_of, request, rows, student, student_b


def run(base, env):
    c = Case("GET /api/materials/{id}/file")
    mine = rows(env, f"""SELECT JSON_OBJECT('id', id, 'size', byte_size) FROM materials
        WHERE class_id={class_of(env, 'student_a1')} ORDER BY id LIMIT 1""")[0]
    status, content = request(base, "GET", f"/api/materials/{mine['id']}/file", token=student(base, env), raw=True)
    c.eq("本班材料可以下载", status, 200)
    c.eq("文件大小与库里记录一致", len(content), mine["size"])
    other = student_b(base, env)
    cross = request(base, "GET", f"/api/materials/{mine['id']}/file", token=other)
    missing = request(base, "GET", f"/api/materials/{MISSING}/file", token=other)
    c.eq("他班材料返回 404", cross[0], 404)
    c.eq("他班材料和不存在的材料响应完全一样", cross, missing)
    return c.result()
