from client import Case, class_of, one, request, sql, student, teacher


def run(base, env):
    c = Case("POST /api/mcp/servers/{id}/connect")
    token = teacher(base, env)
    cls = class_of(env, "teacher_a")
    sid = int(one(env, f"SELECT id FROM mcp_servers WHERE class_id={cls} AND name='campus-tools' ORDER BY id LIMIT 1"))
    status, out = request(base, "POST", f"/api/mcp/servers/{sid}/connect", token=token)
    c.eq("连接返回 200", status, 200)
    c.eq("库里标记为已连接", one(env, f"SELECT connected FROM mcp_servers WHERE id={sid}"), "1")
    tools = [r[0] for r in sql(env, f"SELECT name FROM mcp_tools WHERE server_id={sid} ORDER BY name")]
    first = list(tools)
    status, out = request(base, "POST", f"/api/mcp/servers/{sid}/connect", token=token)
    c.eq("再连一次仍返回 200", status, 200)
    again = [r[0] for r in sql(env, f"SELECT name FROM mcp_tools WHERE server_id={sid} ORDER BY name")]
    c.eq("连续连接两次清单不翻倍", again, first)
    c.check("发现了 formula_hint 和 unit_convert", set(again) >= {"formula_hint", "unit_convert"}, str(again))

    status, body = request(base, "GET", f"/api/mcp/servers/{sid}/tools", token=token)
    c.eq("查看工具返回 200", status, 200)
    names = [t["name"] for t in body.get("tools") or []]
    c.eq("页面上的工具名就是发现清单", sorted(names), sorted(again))
    for item in body.get("tools") or []:
        c.check(f"{item.get('name')} 有说明", bool(item.get("description")))
        c.check(f"{item.get('name')} 有输入结构", isinstance(item.get("input_schema"), dict))

    status, out = request(base, "POST", f"/api/mcp/servers/{sid}/connect", token=student(base, env))
    c.eq("学生连接返回 403", status, 403)
    return c.result()
