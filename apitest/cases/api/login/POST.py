from client import login, request


def run(base, env):
    token = login(base, "teacher_a", env["SEED_TEACHER_PASSWORD"])
    status, me = request(base, "GET", "/api/me", token=token)
    ok = status == 200 and me.get("role") == "teacher" and me.get("class_name") == "A"
    return {"name": "POST /api/login", "ok": ok, "detail": f"status={status} role={me.get('role')}"}
