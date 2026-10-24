package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"campusclaw/internal/skillfile"
)

type seedSkill struct {
	key   string
	class string
	body  string
}

type seedAgent struct {
	key      string
	class    string
	name     string
	prompt   string
	skills   []string
	material []string
}

func seedStudio(ctx context.Context, conn *sql.DB, mcpToken string) error {
	skills := []seedSkill{
		{key: "seed-skill-a-math", class: "A", body: "名称：数学解题引导\n适用条件：方程,去括号,分数,通分,面积,应用题,未知数,行程\n资料：数学\n工具：knowledge_search\n启用：是\n\n学生正在做数学作业，往往已经写了一步。先看懂他卡在哪一步，只指出下一步该检查什么，例如去括号有没有乘到每一项、分母不同要不要先通分。不要算出最终得数，不要把整道题做完。依据只用标题里带「数学」的材料，并标出材料标题和位置。\n"},
		{key: "seed-skill-a-formula", class: "A", body: "名称：公式提示\n适用条件：公式,面积,提示\n资料：数学\n工具：formula_hint@campus-tools,knowledge_search\n启用：是\n\n学生在问该用哪一类公式。先调用公式提示工具拿到检查步骤，再结合检索到的材料说明下一步看什么。不要给出最终得数。\n"},
		{key: "seed-skill-a-unit", class: "A", body: "名称：单位换算\n适用条件：换算,厘米,毫米,千克\n工具：unit_convert@campus-tools\n启用：是\n\n学生明确要换算米、厘米、毫米、千米、克或千克时，调用单位换算工具，用工具返回的结果回答。不要自己编换算关系。\n"},
		{key: "seed-skill-a-chinese", class: "A", body: "名称：语文解题引导\n适用条件：春晓,默写,阅读,中心,作文,开头,古诗,词语\n资料：语文\n工具：knowledge_search\n启用：是\n\n学生正在背诵、做阅读或改作文。先接住他已经写出的那一句，告诉他往前看哪一句、找哪类句子。不要把诗的下一句、中心句或改好的段落直接写出来。依据只用标题里带「语文」的材料。\n"},
		{key: "seed-skill-a-science", class: "A", body: "名称：科学观察\n适用条件：磁铁,沸腾,结冰,水蒸气,三态\n资料：科学\n工具：knowledge_search\n启用：是\n\n学生在问科学课里观察到的现象。只用检索到的材料回答，并标出处。材料里写了的结论可以直接说。\n"},
		{key: "seed-skill-b-physics", class: "B", body: "名称：物理现象\n适用条件：牛顿,外力,静止,匀速\n资料：牛顿\n工具：knowledge_search\n启用：是\n\n只用本班牛顿定律笔记里的句子回答，并标出处。\n"},
	}
	for _, s := range skills {
		if err := ensureSkill(ctx, conn, s); err != nil {
			return err
		}
	}
	agents := []seedAgent{
		{
			key: "seed-agent-a-math", class: "A", name: "数学答疑",
			prompt:   "你是数学答疑助手。学生还在做题，你只帮他看下一步，不替他交卷。",
			skills:   []string{"seed-skill-a-math", "seed-skill-a-formula", "seed-skill-a-unit"},
			material: []string{"seed-class-a-math"},
		},
		{
			key: "seed-agent-a-science", class: "A", name: "科学助教",
			prompt:   "你是科学课助教。回答要贴近实验和观察，先说现象，再说材料里的原因。",
			skills:   []string{"seed-skill-a-science"},
			material: []string{"seed-class-a-science"},
		},
		{
			key: "seed-agent-a-chinese", class: "A", name: "语文陪练",
			prompt:   "你是语文陪练。语气像同学，不直接把标准答案写完。",
			skills:   []string{"seed-skill-a-chinese"},
			material: []string{"seed-class-a-chinese"},
		},
		{
			key: "seed-agent-b-physics", class: "B", name: "物理答疑",
			prompt:   "你是物理答疑助手。只根据本班笔记解释现象。",
			skills:   []string{"seed-skill-b-physics"},
			material: []string{"seed-class-b-newton"},
		},
	}
	for _, a := range agents {
		if err := ensureAgent(ctx, conn, a); err != nil {
			return err
		}
	}
	if strings.TrimSpace(mcpToken) == "" {
		return nil
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO mcp_servers (class_id, name, base_url, auth_token, connected, created_at)
		SELECT id, 'campus-tools', 'http://mcp:8090', ?, 0, UTC_TIMESTAMP() FROM classes WHERE name='A'
		AND NOT EXISTS (SELECT 1 FROM mcp_servers WHERE name='campus-tools' AND class_id=classes.id)`, mcpToken)
	return err
}

func ensureSkill(ctx context.Context, conn *sql.DB, s seedSkill) error {
	parsed, err := skillfile.Parse(s.body)
	if err != nil {
		return err
	}
	tools, err := json.Marshal(parsed.Tools)
	if err != nil {
		return err
	}
	enabled := 0
	if parsed.Enabled {
		enabled = 1
	}
	var id int64
	err = conn.QueryRowContext(ctx, `SELECT id FROM skills WHERE seed_key=?`, s.key).Scan(&id)
	if err == sql.ErrNoRows {
		_, err = conn.ExecContext(ctx, `INSERT INTO skills
			(name, class_id, trigger_text, description, tools_json, source_text, subject_text, enabled, seed_key)
			SELECT ?, id, ?, ?, ?, ?, ?, ?, ? FROM classes WHERE name=?`,
			parsed.Name, parsed.Trigger, parsed.Description, string(tools), parsed.Source, parsed.Subject, enabled, s.key, s.class)
		return err
	}
	return err
}

func ensureAgent(ctx context.Context, conn *sql.DB, a seedAgent) error {
	now := time.Now().UTC()
	var id int64
	err := conn.QueryRowContext(ctx, `SELECT id FROM tutor_agents WHERE seed_key=?`, a.key).Scan(&id)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	res, err := conn.ExecContext(ctx, `INSERT INTO tutor_agents
		(class_id, name, prompt, enabled, seed_key, created_at, updated_at)
		SELECT id, ?, ?, 1, ?, ?, ? FROM classes WHERE name=?`,
		a.name, a.prompt, a.key, now, now, a.class)
	if err != nil {
		return err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM tutor_agent_skills WHERE agent_id=?`, id); err != nil {
		return err
	}
	for _, key := range a.skills {
		if _, err := conn.ExecContext(ctx, `INSERT INTO tutor_agent_skills (agent_id, skill_id)
			SELECT ?, id FROM skills WHERE seed_key=?`, id, key); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM tutor_agent_materials WHERE agent_id=?`, id); err != nil {
		return err
	}
	for _, key := range a.material {
		if _, err := conn.ExecContext(ctx, `INSERT INTO tutor_agent_materials (agent_id, material_id)
			SELECT ?, id FROM materials WHERE seed_key=?`, id, key); err != nil {
			return err
		}
	}
	return nil
}
