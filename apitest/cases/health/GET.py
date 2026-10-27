from client import Case, request


def run(base, env):
    c = Case("GET /health")
    status, body = request(base, "GET", "/health")
    c.eq("不登录也能访问", status, 200)
    c.eq("返回 status=ok", body.get("status"), "ok")
    return c.result()
