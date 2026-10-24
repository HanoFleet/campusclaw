from client import login, request


def run(base, env):
    token = login(base, "teacher_a", env["SEED_TEACHER_PASSWORD"])
    status, body = request(base, "GET", "/api/agents", token=token)
    names = [item.get("name") for item in body.get("agents", [])]
    ok = status == 200 and "数学答疑" in names and "科学助教" in names
    return {"name": "GET /api/agents", "ok": ok, "detail": "、".join(names)}
