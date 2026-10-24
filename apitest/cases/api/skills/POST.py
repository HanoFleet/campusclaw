from client import login, request


def run(base, env):
    token = login(base, "student_a1", env["SEED_STUDENT_A_PASSWORD"])
    status, body = request(base, "POST", "/api/skills", token=token, body={"source": "名称：不该成功\n适用条件：测试\n启用：是\n\n学生不能上传。\n"})
    teacher = login(base, "teacher_a", env["SEED_TEACHER_PASSWORD"])
    listed, skills = request(base, "GET", "/api/skills", token=teacher)
    names = [item.get("name") for item in skills.get("skills", [])]
    ok = status == 403 and listed == 200 and "数学解题引导" in names
    return {"name": "POST /api/skills", "ok": ok, "detail": f"student={status} {body.get('error')} skills={len(names)}"}
