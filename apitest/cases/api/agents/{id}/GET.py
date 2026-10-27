from client import MISSING, Case, agent_named, request, rows, sql, student, teacher_b


def run(base, env):
    c = Case("GET /api/agents/{id}")
    aid = agent_named(env, "student_a1", "数学答疑")
    status, body = request(base, "GET", f"/api/agents/{aid}", token=student(base, env))
    c.eq("本班应用返回 200", status, 200)
    row = rows(env, f"SELECT JSON_OBJECT('name', name, 'prompt', prompt) FROM tutor_agents WHERE id={aid}")[0]
    c.eq("名称和提示词与库里一致", {"name": body.get("name"), "prompt": body.get("prompt")}, row)
    skills = [int(r[0]) for r in sql(env, f"SELECT skill_id FROM tutor_agent_skills WHERE agent_id={aid} ORDER BY skill_id")]
    materials = [int(r[0]) for r in sql(env, f"SELECT material_id FROM tutor_agent_materials WHERE agent_id={aid} ORDER BY material_id")]
    c.eq("绑定的技能与库里一致", body.get("skill_ids"), skills)
    c.eq("绑定的材料与库里一致", body.get("material_ids"), materials)

    other = teacher_b(base, env)
    cross = request(base, "GET", f"/api/agents/{aid}", token=other)
    missing = request(base, "GET", f"/api/agents/{MISSING}", token=other)
    c.eq("他班教师读取返回 404", cross[0], 404)
    c.eq("他班应用和不存在的应用响应完全一样", cross, missing)
    return c.result()
