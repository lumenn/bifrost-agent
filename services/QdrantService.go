package services

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"github.com/sashabaranov/go-openai"
)

type SearchResult struct {
	Content  string                 `json:"content"`
	Score    float64                `json:"score"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Document struct {
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

type QdrantService struct {
	client       *openai.Client
	qdrantClient *qdrant.Client
	modelName    string
	collection   string
}

func NewQdrantService() (*QdrantService, error) {
	// Initialize OpenAI client for embeddings
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	client := openai.NewClient(apiKey)

	// Initialize Qdrant client - using gRPC port as per docs
	qdrantClient, err := qdrant.NewClient(
		&qdrant.Config{
			Host: "localhost",
			Port: 6334,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	collectionName := "weapons_reports"

	// Check if collection exists
	collections, err := qdrantClient.ListCollections(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	collectionExists := false
	for _, collection := range collections {
		if collection == collectionName {
			collectionExists = true
			break
		}
	}

	// Create collection only if it doesn't exist
	if !collectionExists {
		err = qdrantClient.CreateCollection(context.Background(), &qdrant.CreateCollection{
			CollectionName: collectionName,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     1536, // OpenAI embedding size
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create collection: %w", err)
		}
	}

	return &QdrantService{
		client:       client,
		qdrantClient: qdrantClient,
		modelName:    string(openai.AdaEmbeddingV2),
		collection:   collectionName,
	}, nil
}

func (s *QdrantService) getEmbedding(text string) ([]float32, error) {
	resp, err := s.client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(s.modelName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data received")
	}

	return resp.Data[0].Embedding, nil
}

func (s *QdrantService) IndexDocument(content string, metadata map[string]interface{}) error {
	embedding, err := s.getEmbedding(content)
	if err != nil {
		return err
	}

	_, err = s.qdrantClient.Upsert(context.Background(), &qdrant.UpsertPoints{
		CollectionName: s.collection,
		Points: []*qdrant.PointStruct{
			{
				Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: uuid.New().String()}},
				Vectors: qdrant.NewVectors(embedding...),
				Payload: qdrant.NewValueMap(map[string]interface{}{
					"content":  content,
					"metadata": metadata,
				}),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}

	return nil
}

func (s *QdrantService) Search(query string, limit int) ([]*qdrant.ScoredPoint, error) {
	queryEmbedding, err := s.getEmbedding(query)
	if err != nil {
		return nil, err
	}

	newLimit := uint64(limit)

	return s.qdrantClient.Query(context.Background(), &qdrant.QueryPoints{
		CollectionName: s.collection,
		Query:          qdrant.NewQuery(queryEmbedding...),
		Limit:          &newLimit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
}

func (s *QdrantService) GetDocument(id string, collection string) ([]*qdrant.ScoredPoint, error) {
	return s.qdrantClient.Query(context.Background(), &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQueryID(qdrant.NewID(id)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
}
