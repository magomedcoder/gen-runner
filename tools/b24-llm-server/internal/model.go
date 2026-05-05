package internal

import "github.com/magomedcoder/gen/tools/b24-llm-server/api/pb/b24llmpb"

type GenerationWire struct {
	Temperature   *float32 `json:"temperature"`
	MaxTokens     *int32   `json:"max_tokens"`
	StopSequences []string `json:"stop_sequences"`
}

type AnalyzeRequest struct {
	TaskID                   string               `json:"task_id"`
	TaskTitle                string               `json:"task_title"`
	TaskDescription          string               `json:"task_description"`
	TaskStatus               string               `json:"task_status"`
	TaskDeadline             string               `json:"task_deadline"`
	TaskAssignee             string               `json:"task_assignee"`
	TaskPriority             string               `json:"task_priority"`
	TaskGroupID              string               `json:"task_group_id"`
	TaskCreatedBy            string               `json:"task_created_by"`
	TaskAccomplices          string               `json:"task_accomplices"`
	TaskAuditors             string               `json:"task_auditors"`
	TaskParentID             string               `json:"task_parent_id"`
	TaskTimeEstimate         string               `json:"task_time_estimate"`
	TaskTimeSpent            string               `json:"task_time_spent"`
	TaskTags                 string               `json:"task_tags"`
	Checklist                []ChecklistItem      `json:"checklist"`
	Subtasks                 []SubtaskWire        `json:"subtasks"`
	DependenciesPredecessors []SubtaskWire        `json:"dependencies_predecessors"`
	DependenciesSuccessors   []SubtaskWire        `json:"dependencies_successors"`
	TaskUserFields           []UserFieldWire      `json:"task_user_fields"`
	TaskAttachments          []TaskAttachmentWire `json:"task_attachments"`
	Comments                 []TaskComment        `json:"comments"`
	History                  []ChatMessage        `json:"history"`
	Prompt                   string               `json:"prompt"`
	AnalysisMode             string               `json:"analysis_mode"`
	Generation               *GenerationWire      `json:"generation"`
	OutputFormat             string               `json:"output_format"`
}

type TaskComment struct {
	Author string `json:"author"`
	Text   string `json:"text"`
	Time   string `json:"time"`
}

type ChecklistItem struct {
	Title string `json:"title"`
	Done  string `json:"done"`
}

type SubtaskWire struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type UserFieldWire struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type TaskAttachmentWire struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnalyzeResponse struct {
	Message string `json:"message"`
}

func generationFromPB(g *b24llmpb.Generation) *GenerationWire {
	if g == nil {
		return nil
	}
	out := &GenerationWire{}
	has := false

	if g.Temperature != nil {
		t := g.GetTemperature()
		out.Temperature = &t
		has = true
	}

	if g.MaxTokens != nil {
		m := g.GetMaxTokens()
		out.MaxTokens = &m
		has = true
	}

	if len(g.StopSequences) > 0 {
		out.StopSequences = append([]string(nil), g.StopSequences...)
		has = true
	}

	if !has {
		return nil
	}

	return out
}

func pbToAnalyze(pb *b24llmpb.AnalyzeRequest) AnalyzeRequest {
	if pb == nil {
		return AnalyzeRequest{}
	}

	req := AnalyzeRequest{
		TaskID:           pb.GetTaskId(),
		TaskTitle:        pb.GetTaskTitle(),
		TaskDescription:  pb.GetTaskDescription(),
		TaskStatus:       pb.GetTaskStatus(),
		TaskDeadline:     pb.GetTaskDeadline(),
		TaskAssignee:     pb.GetTaskAssignee(),
		TaskPriority:     pb.GetTaskPriority(),
		TaskGroupID:      pb.GetTaskGroupId(),
		TaskCreatedBy:    pb.GetTaskCreatedBy(),
		TaskAccomplices:  pb.GetTaskAccomplices(),
		TaskAuditors:     pb.GetTaskAuditors(),
		TaskParentID:     pb.GetTaskParentId(),
		TaskTimeEstimate: pb.GetTaskTimeEstimate(),
		TaskTimeSpent:    pb.GetTaskTimeSpent(),
		TaskTags:         pb.GetTaskTags(),
		Prompt:           pb.GetPrompt(),
		AnalysisMode:     pb.GetAnalysisMode(),
		OutputFormat:     pb.GetOutputFormat(),
		Generation:       generationFromPB(pb.GetGeneration()),
	}

	for _, c := range pb.GetComments() {
		if c == nil {
			continue
		}
		req.Comments = append(req.Comments, TaskComment{
			Author: c.GetAuthor(),
			Text:   c.GetText(),
			Time:   c.GetTime(),
		})
	}

	for _, it := range pb.GetChecklist() {
		if it == nil {
			continue
		}
		req.Checklist = append(req.Checklist, ChecklistItem{
			Title: it.GetTitle(),
			Done:  it.GetDone(),
		})
	}

	appendSub := func(dst *[]SubtaskWire, src []*b24llmpb.SubtaskWire) {
		for _, st := range src {
			if st == nil {
				continue
			}
			*dst = append(*dst, SubtaskWire{
				ID:     st.GetId(),
				Title:  st.GetTitle(),
				Status: st.GetStatus(),
			})
		}
	}
	appendSub(&req.Subtasks, pb.GetSubtasks())
	appendSub(&req.DependenciesPredecessors, pb.GetDependenciesPredecessors())
	appendSub(&req.DependenciesSuccessors, pb.GetDependenciesSuccessors())

	for _, uf := range pb.GetTaskUserFields() {
		if uf == nil {
			continue
		}
		req.TaskUserFields = append(req.TaskUserFields, UserFieldWire{
			Field: uf.GetField(),
			Value: uf.GetValue(),
		})
	}

	for _, at := range pb.GetTaskAttachments() {
		if at == nil {
			continue
		}
		req.TaskAttachments = append(req.TaskAttachments, TaskAttachmentWire{
			ID:   at.GetId(),
			Name: at.GetName(),
		})
	}

	for _, h := range pb.GetHistory() {
		if h == nil {
			continue
		}
		req.History = append(req.History, ChatMessage{
			Role:    h.GetRole(),
			Content: h.GetContent(),
		})
	}

	return req
}

func pbToSummarizeBatch(pb *b24llmpb.SummarizeBatchRequest) SummarizeBatchRequest {
	if pb == nil {
		return SummarizeBatchRequest{}
	}

	out := SummarizeBatchRequest{
		Generation: generationFromPB(pb.GetGeneration()),
	}

	for _, it := range pb.GetItems() {
		out.Items = append(out.Items, pbToAnalyze(it))
	}

	return out
}

func summarizeBatchToPB(res SummarizeBatchResponse) *b24llmpb.SummarizeBatchResponse {
	pbRes := &b24llmpb.SummarizeBatchResponse{}
	for _, r := range res.Results {
		pbRes.Results = append(pbRes.Results, &b24llmpb.SummarizeBatchResult{
			TaskId:  r.TaskID,
			Message: r.Message,
			Error:   r.Error,
		})
	}

	return pbRes
}
