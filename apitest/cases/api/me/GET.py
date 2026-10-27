from client import Case, q, request, rows, student, teacher_b


def run(base, env):
    c = Case("GET /api/me")
    for name, token in [("student_a1", student(base, env)), ("teacher_b", teacher_b(base, env))]:
        status, body = request(base, "GET", "/api/me", token=token)
        c.eq(f"{name} 返回 200", status, 200)
        want = rows(env, f"""SELECT JSON_OBJECT('id', u.id, 'username', u.username, 'role', u.role,
            'class_id', u.class_id, 'class_name', c.name) FROM users u JOIN classes c ON c.id=u.class_id
            WHERE u.username={q(name)}""")[0]
        c.eq(f"{name} 的身份和班级与库里一致", body, want)
    status, body = request(base, "GET", "/api/me")
    c.eq("不带令牌返回 401", status, 401)
    status, body = request(base, "GET", "/api/me", token="not-a-token")
    c.eq("伪造令牌返回 401", status, 401)
    return c.result()
