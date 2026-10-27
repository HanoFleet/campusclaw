from client import Case, drop_skills, one, request, rows, student, teacher, uniq


def run(base, env):
    c = Case("POST /api/skills")
    token = teacher(base, env)
    name = uniq("通分")[:12]
    src = (
        f"名称：{name}\n适用条件：分母,通分\n资料：数学\n工具：knowledge_search\n启用：是\n\n"
        "学生在做分数加减。先提醒看分母，不要算出结果。\n"
    )
    created = []
    try:
        status, out = request(base, "POST", "/api/skills", token=token, body={"source": src})
        c.eq("上传返回 201", status, 201)
        created.append(out.get("id"))
        row = rows(env, f"SELECT JSON_OBJECT('name', name, 'enabled', enabled, 'description', description) FROM skills WHERE id={int(out.get('id', 0))}")
        c.eq("库里名称、启用和说明与提交一致", row, [{"name": name, "enabled": 1, "description": "学生在做分数加减。先提醒看分母，不要算出结果。"}])

        before = one(env, "SELECT COUNT(*) FROM skills")
        status, out = request(base, "POST", "/api/skills", token=token, body={"source": src})
        c.eq("同班重名返回 400 duplicate_name", (status, out.get("error")), (400, "duplicate_name"))
        status, out = request(base, "POST", "/api/skills", token=token, body={"source": "名称：缺空行\n适用条件：x\n启用：是\n没有空行"})
        c.eq("缺空行返回 400", status, 400)
        status, out = request(base, "POST", "/api/skills", token=token, body={
            "source": f"名称：{uniq('坏工具')[:12]}\n适用条件：x\n工具：not-a-tool\n启用：是\n\n说明\n",
        })
        c.eq("非法工具名返回 400", status, 400)
        status, out = request(base, "POST", "/api/skills", token=token, body={
            "source": f"名称：{uniq('未发现')[:12]}\n适用条件：x\n工具：ghost@campus-tools\n启用：是\n\n说明\n",
        })
        c.eq("引用清单里没有的工具返回 400", status, 400)
        status, out = request(base, "POST", "/api/skills", token=student(base, env), body={"source": src})
        c.eq("学生上传返回 403", status, 403)
        c.eq("被拒的请求都没有入库", one(env, "SELECT COUNT(*) FROM skills"), before)
    finally:
        drop_skills(env, created)
    return c.result()
