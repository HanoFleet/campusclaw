from client import login, request


def run(base, env):
    teacher = login(base, "teacher_a", env["SEED_TEACHER_PASSWORD"])
    student = login(base, "student_a1", env["SEED_STUDENT_A_PASSWORD"])
    _, agents = request(base, "GET", "/api/agents", token=teacher)
    agent_id = next(item["id"] for item in agents["agents"] if item["name"] == "数学答疑")
    status, created = request(base, "POST", "/api/conversations", token=teacher, body={"agent_id": agent_id})
    other, _ = request(base, "GET", f"/api/conversations/{created.get('id')}", token=student)
    ok = status == 201 and created.get("title") == "新的提问" and other == 404
    return {"name": "POST /api/conversations", "ok": ok, "detail": f"create={status} other={other}"}
