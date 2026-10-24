import { FormEvent, useEffect, useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { ApiError, clearToken, getMe, getToken, type Me, api } from "./api";

type Page = "apps" | "skills" | "tools" | "chat";

type Agent = { id: number; name: string; prompt: string; enabled: boolean; skill_ids?: number[]; material_ids?: number[] };
type Skill = {
  id: number;
  name: string;
  trigger_text: string;
  subject_text: string;
  description: string;
  tools: string[];
  enabled: boolean;
  source_text?: string;
};
type Material = { id: number; title: string };
type Server = { id: number; name: string; base_url: string; connected: boolean; last_error: string; has_token: boolean };
type Audit = { id: number; ref: string; status: string; detail: string; username: string; created_at: string };
type Conv = { id: number; title: string; agent_id: number; agent_name: string };
type Msg = { id?: number; role: string; content: string; skill_name?: string; citations?: Cite[]; trace?: Trace };
type Cite = { title?: string; chunk_index?: number; excerpt?: string };
type Trace = {
  prompt_template?: string;
  prompt_template_included?: boolean;
  knowledge?: { sent_full_text?: boolean; hit_count?: number; bound_material_count?: number; note?: string };
  skill?: { selected?: string | null; sent_full_skill_file?: boolean; other_bound_skills_omitted?: string[]; description_chars?: number };
  skill_execution?: string;
  mcp?: { note?: string; declarations_sent?: unknown[]; calls?: { ref: string; status: string; detail?: string }[] };
  messages?: { role: string; content: string }[];
  tools?: unknown[];
};

export function StudioGate({ page, push }: { page: Page; push: (text: string) => void }) {
  const navigate = useNavigate();
  const [me, setMe] = useState<Me | null | undefined>(undefined);
  useEffect(() => {
    getMe().then(setMe).catch(() => setMe(null));
  }, []);
  if (me === undefined) return <p className="boot">正在确认会话…</p>;
  if (me === null) return <Navigate to="/login" replace />;
  const logout = () => {
    void (async () => {
      try {
        await api("/api/logout", { method: "POST" });
      } catch {
        /* 回到登录页 */
      }
      clearToken();
      navigate("/login", { replace: true });
    })();
  };
  return (
    <div className="app-shell">
      <header className="top">
        <div>
          <p className="ask-kicker">CAMPUSCLAW</p>
          <strong>{me.class_name}班 · {me.role === "teacher" ? "教师" : "学生"}</strong>
        </div>
        <div className="top-actions">
          <button type="button" onClick={() => navigate("/materials")}>材料</button>
          <button type="button" className={page === "apps" ? "nav-ask" : ""} onClick={() => navigate("/apps")}>应用</button>
          <button type="button" className={page === "skills" ? "nav-ask" : ""} onClick={() => navigate("/skills")}>技能</button>
          <button type="button" className={page === "tools" ? "nav-ask" : ""} onClick={() => navigate("/tools")}>工具</button>
          <button type="button" className={page === "chat" ? "nav-ask" : ""} onClick={() => navigate("/chat")}>对话</button>
          <button type="button" onClick={logout}>退出</button>
        </div>
      </header>
      {page === "apps" ? <AppsPage me={me} push={push} /> : null}
      {page === "skills" ? <SkillsPage me={me} push={push} /> : null}
      {page === "tools" ? <ToolsPage me={me} push={push} /> : null}
      {page === "chat" ? <ChatPage me={me} push={push} /> : null}
    </div>
  );
}

function AppsPage({ me, push }: { me: Me; push: (text: string) => void }) {
  const navigate = useNavigate();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [materials, setMaterials] = useState<Material[]>([]);
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [skillIDs, setSkillIDs] = useState<number[]>([]);
  const [materialIDs, setMaterialIDs] = useState<number[]>([]);

  async function load() {
    const [a, s, m] = await Promise.all([
      api<{ agents: Agent[] }>("/api/agents"),
      api<{ skills: Skill[] }>("/api/skills"),
      api<{ materials: Material[] }>("/api/materials"),
    ]);
    setAgents(a.agents ?? []);
    setSkills(s.skills ?? []);
    setMaterials(m.materials ?? []);
  }
  useEffect(() => {
    load().catch((err) => push(messageOf(err)));
  }, []);

  async function create(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/agents", {
        method: "POST",
        body: JSON.stringify({ name, prompt, enabled: true, skill_ids: skillIDs, material_ids: materialIDs }),
      });
      setName("");
      setPrompt("");
      await load();
      push("应用已保存");
    } catch (err) {
      push(messageOf(err));
    }
  }

  async function start(agent: Agent) {
    const created = await api<{ id: number }>("/api/conversations", {
      method: "POST",
      body: JSON.stringify({ agent_id: agent.id }),
    });
    navigate(`/chat?id=${created.id}`);
  }

  return (
    <main className="studio">
      <p className="ask-lede">一个班可以有多个应用。每个应用有自己的提示词，并绑定能用的技能和能查的材料。</p>
      <div className="studio-grid">
        {agents.map((agent) => (
          <article className="ask-card" key={agent.id}>
            <h2>{agent.name}</h2>
            <p>{agent.prompt || "没有填写提示词，回答时会用默认角色。"}</p>
            <p className="muted">{agent.enabled ? "已启用" : "已停用"}</p>
            <button className="primary" type="button" onClick={() => void start(agent)} disabled={!agent.enabled}>
              用这个对话
            </button>
          </article>
        ))}
      </div>
      {me.role === "teacher" ? (
        <form className="ask-card studio-form" onSubmit={create}>
          <h2>新建应用</h2>
          <label>名称<input value={name} onChange={(e) => setName(e.target.value)} required maxLength={64} /></label>
          <label>提示词<textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={4} maxLength={2000} placeholder="这个助手是谁、用什么口气说话" /></label>
          <fieldset>
            <legend>绑定技能</legend>
            {skills.map((skill) => (
              <label key={skill.id}>
                <input type="checkbox" checked={skillIDs.includes(skill.id)} onChange={() => setSkillIDs(toggle(skillIDs, skill.id))} />
                {skill.name}
              </label>
            ))}
          </fieldset>
          <fieldset>
            <legend>绑定材料</legend>
            {materials.map((item) => (
              <label key={item.id}>
                <input type="checkbox" checked={materialIDs.includes(item.id)} onChange={() => setMaterialIDs(toggle(materialIDs, item.id))} />
                {item.title}
              </label>
            ))}
          </fieldset>
          <button className="primary" type="submit">保存</button>
        </form>
      ) : null}
    </main>
  );
}

function SkillsPage({ me, push }: { me: Me; push: (text: string) => void }) {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [source, setSource] = useState("名称：实验提示\n适用条件：实验,观察\n资料：科学\n工具：knowledge_search\n启用：是\n\n先让学生说出看到的现象，再引用材料里的句子。不要编造材料里没有的结论。\n");

  async function load() {
    const data = await api<{ skills: Skill[] }>("/api/skills");
    setSkills(data.skills ?? []);
  }
  useEffect(() => {
    load().catch((err) => push(messageOf(err)));
  }, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/skills", { method: "POST", body: JSON.stringify({ source }) });
      await load();
      push("技能已上传");
    } catch (err) {
      push(messageOf(err));
    }
  }

  async function uploadFile(file: File) {
    const text = await file.text();
    setSource(text);
    try {
      await api("/api/skills", { method: "POST", body: JSON.stringify({ source: text }) });
      await load();
      push("技能文件已保存");
    } catch (err) {
      push(messageOf(err));
    }
  }

  return (
    <main className="studio">
      <p className="ask-lede">技能是一份文本：空行上面写名称和适用条件，空行下面写引导说明。内置技能已经放进本班，也可以再上传。</p>
      <div className="studio-grid">
        {skills.map((skill) => (
          <article className="ask-card" key={skill.id}>
            <h2>{skill.name}</h2>
            <p>适用条件：{skill.trigger_text}</p>
            <p>资料范围：{skill.subject_text || "不限"}</p>
            <p>工具：{(skill.tools ?? []).join("，")}</p>
            <p>{skill.description}</p>
          </article>
        ))}
      </div>
      {me.role === "teacher" ? (
        <form className="ask-card studio-form" onSubmit={save}>
          <h2>上传技能</h2>
          <textarea value={source} onChange={(e) => setSource(e.target.value)} rows={10} aria-label="技能全文" />
          <div className="top-actions">
            <button className="primary" type="submit">保存这段文字</button>
            <label className="upload-btn">
              选择文件
              <input type="file" accept=".txt,.md,text/plain" hidden onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                if (file) void uploadFile(file);
              }} />
            </label>
          </div>
        </form>
      ) : null}
    </main>
  );
}

function ToolsPage({ me, push }: { me: Me; push: (text: string) => void }) {
  const [servers, setServers] = useState<Server[]>([]);
  const [audits, setAudits] = useState<Audit[]>([]);
  const [tools, setTools] = useState<Record<number, { name: string; description: string }[]>>({});
  const [name, setName] = useState("campus-tools");
  const [base, setBase] = useState("http://mcp:8090");
  const [token, setToken] = useState("");

  async function load() {
    const data = await api<{ servers: Server[] }>("/api/mcp/servers");
    setServers(data.servers ?? []);
    if (me.role === "teacher") {
      const audit = await api<{ audits: Audit[] }>("/api/mcp/audits");
      setAudits(audit.audits ?? []);
    }
  }
  useEffect(() => {
    load().catch((err) => push(messageOf(err)));
  }, []);

  async function create(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/mcp/servers", { method: "POST", body: JSON.stringify({ name, base_url: base, token }) });
      setToken("");
      await load();
      push("服务器已登记，令牌只留在服务端");
    } catch (err) {
      push(messageOf(err));
    }
  }

  async function connect(id: number, on: boolean) {
    try {
      await api(`/api/mcp/servers/${id}/${on ? "connect" : "disconnect"}`, { method: "POST" });
      await load();
      if (!on) setTools((prev) => ({ ...prev, [id]: [] }));
    } catch (err) {
      push(messageOf(err));
    }
  }

  async function showTools(id: number) {
    const data = await api<{ tools: { name: string; description: string }[] }>(`/api/mcp/servers/${id}/tools`);
    setTools((prev) => ({ ...prev, [id]: data.tools ?? [] }));
  }

  return (
    <main className="studio">
      <p className="ask-lede">外部工具通过 MCP 接入。连接时服务端去握手并发现工具。页面上看不到令牌。</p>
      {servers.map((server) => (
        <article className="ask-card" key={server.id}>
          <h2>{server.name}</h2>
          <p>{server.base_url}</p>
          <p>{server.connected ? "已连接" : "未连接"} · {server.has_token ? "已配置令牌" : "没有令牌"}</p>
          {server.last_error ? <p>{server.last_error}</p> : null}
          {me.role === "teacher" ? (
            <div className="top-actions">
              <button className="primary" type="button" onClick={() => void connect(server.id, true)}>连接</button>
              <button type="button" onClick={() => void connect(server.id, false)}>断开</button>
              <button type="button" onClick={() => void showTools(server.id)}>查看工具</button>
            </div>
          ) : null}
          {(tools[server.id] ?? []).map((tool) => (
            <p key={tool.name}><strong>{tool.name}</strong> {tool.description}</p>
          ))}
        </article>
      ))}
      {me.role === "teacher" ? (
        <form className="ask-card studio-form" onSubmit={create}>
          <h2>登记服务器</h2>
          <label>名称<input value={name} onChange={(e) => setName(e.target.value)} required /></label>
          <label>地址<input value={base} onChange={(e) => setBase(e.target.value)} required /></label>
          <label>令牌<input type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="留空则使用服务端环境变量" /></label>
          <button className="primary" type="submit">登记</button>
        </form>
      ) : null}
      {me.role === "teacher" ? (
        <section className="ask-card">
          <h2>最近调用</h2>
          {audits.length === 0 ? <p>还没有调用记录。</p> : null}
          {audits.map((item) => (
            <p key={item.id}>{item.created_at} · {item.username || "未知用户"} · {item.ref} · {item.status} · {item.detail}</p>
          ))}
        </section>
      ) : null}
    </main>
  );
}

function ChatPage({ push }: { me: Me; push: (text: string) => void }) {
  const [params, setParams] = useSearchParams();
  const [convs, setConvs] = useState<Conv[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentID, setAgentID] = useState(0);
  const [messages, setMessages] = useState<Msg[]>([]);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const current = Number(params.get("id") || 0);

  async function loadList() {
    const [c, a] = await Promise.all([
      api<{ conversations: Conv[] }>("/api/conversations"),
      api<{ agents: Agent[] }>("/api/agents"),
    ]);
    setConvs(c.conversations ?? []);
    const enabled = (a.agents ?? []).filter((item) => item.enabled);
    setAgents(enabled);
    if (!agentID && enabled[0]) setAgentID(enabled[0].id);
  }
  useEffect(() => {
    loadList().catch((err) => push(messageOf(err)));
  }, []);

  useEffect(() => {
    if (!current) {
      setMessages([]);
      return;
    }
    api<{ messages: Msg[] }>(`/api/conversations/${current}`)
      .then((data) => setMessages(data.messages ?? []))
      .catch((err) => push(messageOf(err)));
  }, [current]);

  async function createConv(event: FormEvent) {
    event.preventDefault();
    const created = await api<{ id: number }>("/api/conversations", {
      method: "POST",
      body: JSON.stringify({ agent_id: agentID }),
    });
    await loadList();
    setParams({ id: String(created.id) });
  }

  async function ask(event: FormEvent) {
    event.preventDefault();
    if (!current || !text.trim()) return;
    const query = text.trim();
    setText("");
    setBusy(true);
    setMessages((prev) => [...prev, { role: "user", content: query }, { role: "assistant", content: "" }]);
    try {
      const headers = new Headers({ "Content-Type": "application/json" });
      const token = getToken();
      if (token) headers.set("Authorization", `Bearer ${token}`);
      const res = await fetch(`/api/conversations/${current}/messages`, {
        method: "POST",
        headers,
        credentials: "include",
        body: JSON.stringify({ query }),
      });
      if (!res.ok || !res.body) {
        const raw = await res.text();
        throw new Error(raw || "提问失败");
      }
      await readSSE(res, (eventName, data) => {
        if (eventName === "delta") {
          const piece = String((data as { text?: string }).text ?? "");
          setMessages((prev) => {
            const next = [...prev];
            const last = next[next.length - 1];
            if (last && last.role === "assistant") next[next.length - 1] = { ...last, content: last.content + piece };
            return next;
          });
        }
        if (eventName === "meta") {
          const meta = data as { skill?: { name?: string }; citations?: Cite[] };
          setMessages((prev) => {
            const next = [...prev];
            const last = next[next.length - 1];
            if (last && last.role === "assistant") {
              next[next.length - 1] = { ...last, skill_name: meta.skill?.name, citations: meta.citations };
            }
            return next;
          });
        }
        if (eventName === "done") {
          const done = data as { trace?: Trace };
          setMessages((prev) => {
            const next = [...prev];
            const last = next[next.length - 1];
            if (last && last.role === "assistant") next[next.length - 1] = { ...last, trace: done.trace };
            return next;
          });
        }
        if (eventName === "error") push("回答中断了");
      });
      await loadList();
    } catch (err) {
      push(err instanceof Error ? err.message : "提问失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="studio chat-layout">
      <aside>
        <form onSubmit={createConv} className="studio-form">
          <label>
            选择应用
            <select value={agentID} onChange={(e) => setAgentID(Number(e.target.value))}>
              {agents.map((agent) => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
            </select>
          </label>
          <button className="primary" type="submit">新对话</button>
        </form>
        {convs.map((conv) => (
          <button key={conv.id} type="button" className={conv.id === current ? "conv active" : "conv"} onClick={() => setParams({ id: String(conv.id) })}>
            <strong>{conv.title}</strong>
            <span>{conv.agent_name}</span>
          </button>
        ))}
      </aside>
      <section>
        {!current ? <p className="ask-lede">先选一个应用，再开始对话。</p> : null}
        {messages.map((msg, index) => (
          <article key={`${msg.role}-${index}`} className={msg.role === "user" ? "ask-user" : "ask-card"}>
            {msg.role === "assistant" ? <h2>回答{msg.skill_name ? ` · ${msg.skill_name}` : ""}</h2> : null}
            <p>{msg.content}</p>
            {msg.citations?.length ? (
              <ol>
                {msg.citations.map((cite, i) => (
                  <li key={i}>[{i + 1}] {cite.title} · 切片 {cite.chunk_index}<br />{cite.excerpt}</li>
                ))}
              </ol>
            ) : null}
            {msg.trace ? <TraceView trace={msg.trace} /> : null}
          </article>
        ))}
        {current ? (
          <form className="ask-composer" onSubmit={ask}>
            <input value={text} onChange={(e) => setText(e.target.value)} placeholder="向这个应用提问" disabled={busy} />
            <button className="primary" type="submit" disabled={busy || text.trim().length < 2}>发送</button>
          </form>
        ) : null}
      </section>
    </main>
  );
}

function TraceView({ trace }: { trace: Trace }) {
  const skill = trace.skill ?? {};
  const knowledge = trace.knowledge ?? {};
  const mcp = trace.mcp ?? {};
  return (
    <details className="trace" open>
      <summary>这次发给模型的内容</summary>
      <p>提示词模板会放进系统消息。本次{trace.prompt_template_included ? "已带上" : "没有带上"}：{trace.prompt_template}</p>
      <p>知识库没有整库传递。绑定了 {knowledge.bound_material_count ?? 0} 份材料，实际送进去 {knowledge.hit_count ?? 0} 条检索片段。全文传递：{String(knowledge.sent_full_text)}</p>
      <p>技能全文传递：{String(skill.sent_full_skill_file)}。选中的是 {skill.selected || "无"}，只传了这条说明（{skill.description_chars ?? 0} 字）。没有选中的技能：{(skill.other_bound_skills_omitted ?? []).join("、") || "无"}。</p>
      <p>{trace.skill_execution}</p>
      <p>{mcp.note}</p>
      {(mcp.calls ?? []).map((call) => (
        <p key={call.ref}>工具 {call.ref} · {call.status} · {call.detail}</p>
      ))}
    </details>
  );
}

async function readSSE(res: Response, onEvent: (name: string, data: unknown) => void) {
  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx = buf.indexOf("\n\n");
    while (idx >= 0) {
      const block = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      let name = "message";
      const data: string[] = [];
      for (const line of block.split("\n")) {
        if (line.startsWith("event:")) name = line.slice(6).trim();
        if (line.startsWith("data:")) data.push(line.slice(5).trim());
      }
      if (data.length) onEvent(name, JSON.parse(data.join("\n")));
      idx = buf.indexOf("\n\n");
    }
  }
}

function toggle(list: number[], id: number) {
  return list.includes(id) ? list.filter((item) => item !== id) : [...list, id];
}

function messageOf(err: unknown) {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "请求失败";
}
