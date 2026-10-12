package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type ChatbotRepo struct {
	db *sql.DB
}

func NewChatbotRepo(db *sql.DB) *ChatbotRepo {
	return &ChatbotRepo{db: db}
}

func (r *ChatbotRepo) Create(ctx context.Context, b ChatbotInstance) error {
	ragIDsJSON, err := marshalJSON(b.RAGCollectionIDs)
	if err != nil {
		return fmt.Errorf("marshal chatbot rag_collection_ids: %w", err)
	}
	settingsJSON, err := marshalJSON(nonNilSettings(b.Settings))
	if err != nil {
		return fmt.Errorf("marshal chatbot settings: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO chatbot_instances(
			id, name, description, prompt_template_id, provider_id, model,
			rag_collection_ids, settings_json, external_id, is_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, b.ID, b.Name, nullableString(b.Description), nullableString(b.PromptTemplateID),
		nullableString(b.ProviderID), b.Model, ragIDsJSON, settingsJSON, b.ExternalID, boolToInt(b.IsEnabled))
	if err != nil {
		return fmt.Errorf("create chatbot: %w", err)
	}
	return nil
}

func (r *ChatbotRepo) List(ctx context.Context) ([]ChatbotInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, prompt_template_id, provider_id, model,
		       rag_collection_ids, settings_json, external_id, is_enabled,
		       created_at, updated_at
		FROM chatbot_instances
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list chatbots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ChatbotInstance{}
	for rows.Next() {
		b, err := scanChatbot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chatbots: %w", err)
	}
	return out, nil
}

func (r *ChatbotRepo) GetByID(ctx context.Context, id string) (ChatbotInstance, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, prompt_template_id, provider_id, model,
		       rag_collection_ids, settings_json, external_id, is_enabled,
		       created_at, updated_at
		FROM chatbot_instances
		WHERE id = ?
	`, id)
	return scanChatbot(row)
}

func (r *ChatbotRepo) GetByExternalID(ctx context.Context, externalID string) (ChatbotInstance, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, prompt_template_id, provider_id, model,
		       rag_collection_ids, settings_json, external_id, is_enabled,
		       created_at, updated_at
		FROM chatbot_instances
		WHERE external_id = ?
	`, externalID)
	return scanChatbot(row)
}

func (r *ChatbotRepo) Update(ctx context.Context, b ChatbotInstance) error {
	ragIDsJSON, err := marshalJSON(b.RAGCollectionIDs)
	if err != nil {
		return fmt.Errorf("marshal chatbot rag_collection_ids: %w", err)
	}
	settingsJSON, err := marshalJSON(nonNilSettings(b.Settings))
	if err != nil {
		return fmt.Errorf("marshal chatbot settings: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE chatbot_instances
		SET name = ?,
		    description = ?,
		    prompt_template_id = ?,
		    provider_id = ?,
		    model = ?,
		    rag_collection_ids = ?,
		    settings_json = ?,
		    external_id = ?,
		    is_enabled = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, b.Name, nullableString(b.Description), nullableString(b.PromptTemplateID),
		nullableString(b.ProviderID), b.Model, ragIDsJSON, settingsJSON, b.ExternalID,
		boolToInt(b.IsEnabled), b.ID)
	if err != nil {
		return fmt.Errorf("update chatbot: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ChatbotRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM chatbot_instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete chatbot: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ChatbotRepo) CreateConversation(ctx context.Context, c ChatbotConversation) error {
	metadataJSON, err := marshalJSON(nonNilSettings(c.Metadata))
	if err != nil {
		return fmt.Errorf("marshal chatbot conversation metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO chatbot_conversations(
			id, chatbot_id, api_key_id, external_user_id, title, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?)
	`, c.ID, c.ChatbotID, nullableInt64(c.APIKeyID), nullableString(c.ExternalUserID),
		nullableString(c.Title), metadataJSON)
	if err != nil {
		return fmt.Errorf("create chatbot conversation: %w", err)
	}
	return nil
}

func (r *ChatbotRepo) GetConversation(ctx context.Context, id string) (ChatbotConversation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, chatbot_id, api_key_id, external_user_id, title,
		       metadata_json, created_at, updated_at
		FROM chatbot_conversations
		WHERE id = ?
	`, id)
	return scanChatbotConversation(row)
}

func (r *ChatbotRepo) AddMessage(ctx context.Context, m ChatbotMessage) error {
	metadataJSON, err := marshalJSON(nonNilSettings(m.Metadata))
	if err != nil {
		return fmt.Errorf("marshal chatbot message metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO chatbot_messages(
			id, conversation_id, role, content, model,
			prompt_tokens, completion_tokens, total_tokens, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.ID, m.ConversationID, m.Role, m.Content, nullableString(m.Model),
		m.PromptTokens, m.CompletionTokens, m.TotalTokens, metadataJSON)
	if err != nil {
		return fmt.Errorf("add chatbot message: %w", err)
	}
	return nil
}

func (r *ChatbotRepo) ListMessages(ctx context.Context, conversationID string) ([]ChatbotMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, model,
		       prompt_tokens, completion_tokens, total_tokens,
		       metadata_json, created_at
		FROM chatbot_messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC
	`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list chatbot messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ChatbotMessage{}
	for rows.Next() {
		m, err := scanChatbotMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chatbot messages: %w", err)
	}
	return out, nil
}

type chatbotScanner interface {
	Scan(dest ...any) error
}

func scanChatbot(s chatbotScanner) (ChatbotInstance, error) {
	var b ChatbotInstance
	var desc, templateID, providerID sql.NullString
	var ragIDsRaw, settingsRaw string
	var enabled int
	var createdAtRaw, updatedAtRaw string
	if err := s.Scan(
		&b.ID,
		&b.Name,
		&desc,
		&templateID,
		&providerID,
		&b.Model,
		&ragIDsRaw,
		&settingsRaw,
		&b.ExternalID,
		&enabled,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return ChatbotInstance{}, err
	}
	if desc.Valid {
		b.Description = desc.String
	}
	if templateID.Valid {
		b.PromptTemplateID = templateID.String
	}
	if providerID.Valid {
		b.ProviderID = providerID.String
	}
	b.RAGCollectionIDs = unmarshalStringList(ragIDsRaw)
	b.Settings = unmarshalMap(settingsRaw)
	b.IsEnabled = enabled == 1
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ChatbotInstance{}, fmt.Errorf("parse chatbot created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return ChatbotInstance{}, fmt.Errorf("parse chatbot updated_at: %w", err)
	}
	b.CreatedAt = createdAt
	b.UpdatedAt = updatedAt
	return b, nil
}

func scanChatbotConversation(s chatbotScanner) (ChatbotConversation, error) {
	var c ChatbotConversation
	var apiKeyID sql.NullInt64
	var externalUserID, title sql.NullString
	var metadataRaw string
	var createdAtRaw, updatedAtRaw string
	if err := s.Scan(
		&c.ID,
		&c.ChatbotID,
		&apiKeyID,
		&externalUserID,
		&title,
		&metadataRaw,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return ChatbotConversation{}, err
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		c.APIKeyID = &v
	}
	if externalUserID.Valid {
		c.ExternalUserID = externalUserID.String
	}
	if title.Valid {
		c.Title = title.String
	}
	c.Metadata = unmarshalMap(metadataRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ChatbotConversation{}, fmt.Errorf("parse conversation created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return ChatbotConversation{}, fmt.Errorf("parse conversation updated_at: %w", err)
	}
	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return c, nil
}

func scanChatbotMessage(s chatbotScanner) (ChatbotMessage, error) {
	var m ChatbotMessage
	var model sql.NullString
	var metadataRaw string
	var createdAtRaw string
	if err := s.Scan(
		&m.ID,
		&m.ConversationID,
		&m.Role,
		&m.Content,
		&model,
		&m.PromptTokens,
		&m.CompletionTokens,
		&m.TotalTokens,
		&metadataRaw,
		&createdAtRaw,
	); err != nil {
		return ChatbotMessage{}, err
	}
	if model.Valid {
		m.Model = model.String
	}
	m.Metadata = unmarshalMap(metadataRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ChatbotMessage{}, fmt.Errorf("parse chatbot message created_at: %w", err)
	}
	m.CreatedAt = createdAt
	return m, nil
}

func nonNilSettings(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}
