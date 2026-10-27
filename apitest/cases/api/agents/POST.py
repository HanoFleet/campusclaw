from client import Case, class_of, drop_agents, one, request, rows, student, teacher, uniq


def run(base, env):
    c = Case("POST /api/agents")
    token = teacher(base, env)
    cls = class_of(env, "teacher_a")
    skill = int(one(env, f"SELECT id FROM skills WHERE class_id={cls} ORDER BY id LIMIT 1"))
    material = int(one(env, f"SELECT id FROM materials WHERE class_id={cls} ORDER BY id LIMIT 1"))
    other_skill = int(one(env, f"SELECT id FROM skills WHERE class_id={class_of(env, 'teacher_b')} ORDER BY id LIMIT 1"))
    name = uniq("接口测试应用")
    created = []
    try:
        body = {"name": name, "prompt": "你是接口测试用的助教。", "skill_ids": [skill], "material_ids": [material]}
        status, out = request(base, "POST", "/api/agents", token=token, body=body)
        c.eq("新建返回 201", status, 201)
        created.append(out.get("id"))
        row = rows(env, f"SELECT JSON_OBJECT('class_id', class_id, 'name', name, 'prompt', prompt, 'enabled', enabled) FROM tutor_agents WHERE id={int(out.get('id', 0))}")
        c.eq("库里的班级、名称、提示词、启用状态正确", row, [{"class_id": cls, "name": name, "prompt": body["prompt"], "enabled": 1}])
        c.eq("库里绑定了这条技能", one(env, f"SELECT GROUP_CONCAT(skill_id) FROM tutor_agent_skills WHERE agent_id={int(out.get('id', 0))}"), str(skill))
        c.eq("库里绑定了这份材料", one(env, f"SELECT GROUP_CONCAT(material_id) FROM tutor_agent_materials WHERE agent_id={int(out.get('id', 0))}"), str(material))

        before = one(env, "SELECT COUNT(*) FROM tutor_agents")
        status, out = request(base, "POST", "/api/agents", token=token, body={"name": name})
        c.eq("同班重名返回 400 duplicate_name", (status, out.get("error")), (400, "duplicate_name"))
        status, out = request(base, "POST", "/api/agents", token=token, body={"name": "长" * 65})
        c.eq("名称超过 64 字返回 400", status, 400)
        status, out = request(base, "POST", "/api/agents", token=token, body={"name": uniq("提示词过长"), "prompt": "长" * 2001})
        c.eq("提示词超过 2000 字返回 400", status, 400)
        status, out = request(base, "POST", "/api/agents", token=token, body={"name": uniq("绑他班"), "skill_ids": [other_skill]})
        c.eq("绑定他班技能返回 404", status, 404)
        status, out = request(base, "POST", "/api/agents", token=student(base, env), body={"name": uniq("学生建")})
        c.eq("学生新建返回 403", status, 403)
        c.eq("被拒的请求都没有入库", one(env, "SELECT COUNT(*) FROM tutor_agents"), before)
    finally:
        drop_agents(env, created)
    return c.result()
