import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, type AskResult, type Me, api } from "./api";

type Turn = {
  question: string;
  result: AskResult;
};

export function AskPage({
  me,
  onLogout,
  push,
}: {
  me: Me;
  onLogout: () => void;
  push: (text: string) => void;
}) {
  const navigate = useNavigate();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [turns, setTurns] = useState<Turn[]>([]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    const q = text.trim();
    if (!q) return;
    setBusy(true);
    try {
      const result = await api<AskResult>("/api/ask", {
        method: "POST",
        body: JSON.stringify({ query: q, class_id: 999 }),
      });
      setTurns((prev) => [...prev, { question: q, result }]);
      setText("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        navigate("/login", { replace: true });
        return;
      }
      push(err instanceof ApiError && err.status === 503 ? "问答服务暂时不可用" : "提问失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="app-shell ask-shell">
      <header className="top">
        <div className="ask-brand">
          <p className="ask-kicker">CAMPUSCLAW</p>
          <h1>{me.class_name}班知识问答</h1>
          <span>
            {me.username} · {me.role === "teacher" ? "教师" : "学生"}
          </span>
        </div>
        <div className="top-actions">
          <button type="button" className="nav-ask" onClick={() => navigate("/materials")}>
            材料
          </button>
          <button type="button" onClick={onLogout}>
            登出
          </button>
        </div>
      </header>
      <div className="ask-tabs" role="tablist">
        <button type="button" onClick={() => navigate("/materials")}>
          知识检索
        </button>
        <button type="button" className="tab-active" aria-current="page">
          知识问答
        </button>
      </div>
      <p className="ask-lede">
        每个问题先在本班做混合检索，取前 4 条切片再生成短回答，并标出 [1]、[2]。没有命中时不调用模型，直接说明资料里没有。
      </p>
      <div className="ask-thread">
        {turns.length === 0 ? <p className="empty">对本班资料提问。没有依据时不会编造出处。</p> : null}
        {turns.map((turn, i) => (
          <article className="ask-turn" key={`${turn.question}-${i}`}>
            <p className="ask-user">{turn.question}</p>
            <section className="ask-card">
              <h2>依据回答</h2>
              <p className="ask-answer">{turn.result.answer}</p>
              {turn.result.citations?.length ? (
                <ol className="ask-cites">
                  {turn.result.citations.map((hit, index) => (
                    <li key={`${hit.material_id}-${hit.chunk_index}-${index}`}>
                      <button type="button" onClick={() => navigate("/materials")}>
                        [{index + 1}] {hit.title} · 切片 {hit.chunk_index} · 偏移 {hit.start}-{hit.end}
                      </button>
                      <em>{hit.excerpt}</em>
                    </li>
                  ))}
                </ol>
              ) : null}
            </section>
          </article>
        ))}
      </div>
      <form className="ask-composer" onSubmit={onSubmit}>
        <input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="你想问本班资料什么？"
          aria-label="知识问答"
          disabled={busy}
        />
        <button className="primary" type="submit" disabled={busy || !text.trim()}>
          {busy ? "检索中…" : "提问"}
        </button>
      </form>
    </div>
  );
}
