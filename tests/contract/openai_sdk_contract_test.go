package contract_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func TestOpenAISDKCompatibility(t *testing.T) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_CONTRACT_BASE_URL")), "/")
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_CONTRACT_API_KEY"))
	if baseURL == "" || apiKey == "" {
		t.Skip("set OPENAI_CONTRACT_BASE_URL and OPENAI_CONTRACT_API_KEY to run the official OpenAI SDK compatibility test")
	}

	chatModel := envDefault("OPENAI_CONTRACT_CHAT_MODEL", "contract-model")
	embeddingModel := envDefault("OPENAI_CONTRACT_EMBEDDING_MODEL", "embed-model")
	responsesModel := envDefault("OPENAI_CONTRACT_RESPONSES_MODEL", chatModel)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	models, err := client.Models.List(ctx)
	if err != nil {
		t.Fatalf("list models through official SDK: %v", err)
	}
	if len(models.Data) == 0 {
		t.Fatalf("expected at least one model from SDK models list")
	}

	chat, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String("Say ok."),
				},
			},
		}},
		Model:       shared.ChatModel(chatModel),
		Temperature: openai.Float(0),
	})
	if err != nil {
		t.Fatalf("chat completions through official SDK: %v", err)
	}
	if chat.ID == "" || len(chat.Choices) == 0 {
		t.Fatalf("unexpected SDK chat response: %#v", chat)
	}

	embedding, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String("embed this"),
		},
		Model: openai.EmbeddingModel(embeddingModel),
	})
	if err != nil {
		t.Fatalf("embeddings through official SDK: %v", err)
	}
	if len(embedding.Data) == 0 || len(embedding.Data[0].Embedding) == 0 {
		t.Fatalf("unexpected SDK embedding response: %#v", embedding)
	}

	response, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Model: shared.ResponsesModel(responsesModel),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String("Say ok."),
		},
	})
	if err != nil {
		t.Fatalf("responses through official SDK: %v", err)
	}
	if response.ID == "" {
		t.Fatalf("unexpected SDK responses response: %#v", response)
	}
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
