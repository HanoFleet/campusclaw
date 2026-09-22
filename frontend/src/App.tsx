import { FormEvent, useEffect, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import { Navigate, Route, Routes, useNavigate } from "react-router-dom";
import remarkGfm from "remark-gfm";
import {
  ApiError,
  getMe,
  type MaterialDetail,
  type MaterialItem,
  type Me,
  api,
} from "./api";

type Toast = { id: number; text: string };

function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([]);
  function push(text: string) {
    const id = Date.now() + Math.random();
    setToasts((prev) => [...prev, { id, text }]);
    window.setTimeout(() => {
      setToasts((prev) => prev.filter((item) => item.id !== id));
    }, 3200);
  }
  return { toasts, push };
}

export default function App() {
  const { toasts, push } = useToasts();
  return (
    <>
      <Routes>
        <Route path="/login" element={<LoginPage push={push} />} />
        <Route path="/materials" element={<MaterialsGate push={push} />} />
        <Route path="*" element={<Navigate to="/materials" replace />} />
      </Routes>
      <div className="toasts" aria-live="polite">
        {toasts.map((toast) => (
          <div className="toast" key={toast.id}>
            {toast.text}
          </div>
        ))}
      </div>
    </>
  );
}

function LoginPage({ push }: { push: (text: string) => void }) {
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getMe()
      .then(() => navigate("/materials", { replace: true }))
      .catch(() => undefined);
  }, [navigate]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      });
      push("已登录");
      navigate("/materials", { replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.status === 503) {
        setError("服务暂时不可用");
        push("服务暂时不可用");
      } else {
        setError("用户名或密码错误");
        push("用户名或密码错误");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-shell">
      <section className="login-card">
        <Brand />
        <h1>登录教研材料库</h1>
        <p className="lede">教师上传本班材料，学生只看本班。班级由服务端会话决定。</p>
        <form onSubmit={onSubmit}>
          <label>
            用户名
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" required />
          </label>
          <label>
            密码
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </label>
          {error ? <p className="form-error">{error}</p> : null}
          <button className="primary" type="submit" disabled={busy}>
            {busy ? "正在登录…" : "登录"}
          </button>
        </form>
      </section>
    </main>
  );
}

function MaterialsGate({ push }: { push: (text: string) => void }) {
  const [me, setMe] = useState<Me | null | undefined>(undefined);
  useEffect(() => {
    getMe()
      .then(setMe)
      .catch(() => setMe(null));
  }, []);
  if (me === undefined) return <p className="boot">正在确认会话…</p>;
  if (me === null) return <Navigate to="/login" replace />;
  return <MaterialsPage me={me} push={push} />;
}

function MaterialsPage({ me, push }: { me: Me; push: (text: string) => void }) {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<MaterialItem[]>([]);
  const [active, setActive] = useState<MaterialDetail | null>(null);
  const [view, setView] = useState<"list" | "grid">(() =>
    localStorage.getItem("campusclaw-view") === "grid" ? "grid" : "list",
  );
  const [theme, setTheme] = useState<"light" | "dark">(
    document.documentElement.dataset.theme === "dark" ? "dark" : "light",
  );
  const [progress, setProgress] = useState<number | null>(null);
  const [paletteOpen, setPaletteOpen] = useState(false);

  useEffect(() => {
    const handle = window.setTimeout(() => {
      const params = new URLSearchParams();
      if (query.trim()) params.set("q", query.trim());
      const suffix = params.toString() ? `?${params}` : "";
      api<{ materials: MaterialItem[] }>(`/api/materials${suffix}`)
        .then((data) => setItems(data.materials))
        .catch((err: unknown) => {
          if (err instanceof ApiError && err.status === 401) {
            navigate("/login", { replace: true });
            return;
          }
          push("材料列表暂时不可用");
        });
    }, 200);
    return () => window.clearTimeout(handle);
  }, [query, navigate, push]);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen(true);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  function toggleTheme() {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.dataset.theme = next;
    localStorage.setItem("campusclaw-theme", next);
  }

  function toggleView() {
    const next = view === "list" ? "grid" : "list";
    setView(next);
    localStorage.setItem("campusclaw-view", next);
  }

  async function openItem(id: number) {
    try {
      const detail = await api<MaterialDetail>(`/api/materials/${id}`);
      setActive(detail);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        navigate("/login", { replace: true });
        return;
      }
      if (err instanceof ApiError && err.status === 404) {
        push("材料不存在");
        return;
      }
      push("无法打开材料");
    }
  }

  async function logout() {
    try {
      await api("/api/logout", { method: "POST" });
    } catch {
      /* 会话已经失效时直接回登录页 */
    }
    navigate("/login", { replace: true });
  }

  function upload(file: File) {
    const form = new FormData();
    form.append("file", file);
    form.append("class_id", "999");
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/materials");
    xhr.withCredentials = true;
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) setProgress(event.loaded / event.total);
    };
    xhr.onload = () => {
      setProgress(null);
      if (xhr.status === 201) {
        push("已写入知识库");
        setQuery((value) => value);
        const created = JSON.parse(xhr.responseText) as { id: number };
        void openItem(created.id);
        api<{ materials: MaterialItem[] }>("/api/materials")
          .then((data) => setItems(data.materials))
          .catch(() => undefined);
        return;
      }
      if (xhr.status === 401) {
        navigate("/login", { replace: true });
        return;
      }
      if (xhr.status === 403) {
        push("没有上传权限");
        return;
      }
      if (xhr.status === 413) {
        push("文件超过大小上限");
        return;
      }
      push("文件未被接受");
    };
    xhr.onerror = () => {
      setProgress(null);
      push("上传失败");
    };
    setProgress(0);
    xhr.send(form);
  }

  const commands = useMemo(() => {
    const list = [
      { id: "search", label: "聚焦搜索", run: () => document.getElementById("material-search")?.focus() },
      { id: "theme", label: theme === "dark" ? "切换到浅色" : "切换到深色", run: toggleTheme },
      { id: "view", label: view === "list" ? "切换到网格" : "切换到列表", run: toggleView },
      { id: "logout", label: "退出登录", run: () => void logout() },
    ];
    if (me.role === "teacher") {
      list.splice(3, 0, {
        id: "upload",
        label: "上传材料",
        run: () => document.getElementById("upload-input")?.click(),
      });
    }
    return list;
    // eslint 不在本项目启用；函数在渲染时重建即可
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [me.role, theme, view]);

  return (
    <div className="app-shell">
      <header className="top">
        <Brand />
        <div className="who">
          <strong>{me.username}</strong>
          <span>
            {me.role === "teacher" ? "教师" : "学生"} · {me.class_name} 班
          </span>
        </div>
        <div className="top-actions">
          <button type="button" onClick={toggleTheme}>
            {theme === "dark" ? "浅色" : "深色"}
          </button>
          <button type="button" onClick={toggleView}>
            {view === "list" ? "网格" : "列表"}
          </button>
          <button type="button" onClick={() => setPaletteOpen(true)}>
            命令
          </button>
          <button type="button" onClick={() => void logout()}>
            退出
          </button>
        </div>
      </header>
      <div className="workspace">
        <section className="list-pane">
          <div className="toolbar">
            <input
              id="material-search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索本班标题或正文"
              aria-label="搜索本班材料"
            />
            {me.role === "teacher" ? (
              <label className="upload-btn">
                上传
                <input
                  id="upload-input"
                  type="file"
                  accept=".txt,.md,text/plain,text/markdown"
                  hidden
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    e.target.value = "";
                    if (file) upload(file);
                  }}
                />
              </label>
            ) : null}
          </div>
          {progress !== null ? (
            <div className="progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(progress * 100)}>
              <span style={{ width: `${Math.round(progress * 100)}%` }} />
            </div>
          ) : null}
          <div className={view === "grid" ? "cards grid" : "cards list"}>
            {items.length === 0 ? <p className="empty">这一班还没有匹配的材料。</p> : null}
            {items.map((item) => (
              <button
                type="button"
                key={item.id}
                className={active?.id === item.id ? "card active" : "card"}
                onClick={() => void openItem(item.id)}
              >
                <strong>{item.title}</strong>
                <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString("zh-CN")}</time>
              </button>
            ))}
          </div>
        </section>
        <article className="detail">
          {active ? (
            <>
              <div className="detail-head">
                <h2>{active.title}</h2>
                <a href={`/api/materials/${active.id}/file`}>下载原文</a>
              </div>
              <div className="markdown">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{active.body}</ReactMarkdown>
              </div>
            </>
          ) : (
            <p className="empty">选一份材料查看正文。正文按 Markdown 渲染，不会执行其中的脚本。</p>
          )}
        </article>
      </div>
      {paletteOpen ? (
        <CommandPalette
          commands={commands}
          onClose={() => setPaletteOpen(false)}
          onRun={(command) => {
            setPaletteOpen(false);
            command.run();
          }}
        />
      ) : null}
    </div>
  );
}

function CommandPalette({
  commands,
  onClose,
  onRun,
}: {
  commands: { id: string; label: string; run: () => void }[];
  onClose: () => void;
  onRun: (command: { id: string; label: string; run: () => void }) => void;
}) {
  const [text, setText] = useState("");
  const filtered = commands.filter((command) => command.label.includes(text.trim()));
  return (
    <div className="palette-backdrop" onClick={onClose}>
      <div
        className="palette"
        role="dialog"
        aria-label="命令面板"
        onClick={(event) => event.stopPropagation()}
      >
        <input
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="输入命令"
          onKeyDown={(event) => {
            if (event.key === "Escape") onClose();
            if (event.key === "Enter" && filtered[0]) onRun(filtered[0]);
          }}
        />
        <ul>
          {filtered.map((command) => (
            <li key={command.id}>
              <button type="button" onClick={() => onRun(command)}>
                {command.label}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function Brand() {
  return (
    <div className="brand">
      <span className="seal">未名</span>
      <div>
        <p>CampusClaw</p>
        <small>教研材料 · 班级边界</small>
      </div>
    </div>
  );
}
