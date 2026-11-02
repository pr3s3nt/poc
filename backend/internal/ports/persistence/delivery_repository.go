package persistence

import (
	"context"
	"orchestrator/internal/domain/delivery"
)

type DeliveryPlatformRepository interface {
	GetPlatformDeliveryDefault(context.Context, string) (delivery.PlatformDefault, error)
	SetPlatformDeliveryDefault(context.Context, delivery.PlatformDefault, int64) (delivery.PlatformDefault, error)
	GetApplicationDeliveryRepository(context.Context, string, string) (delivery.ApplicationRepository, error)
	ReserveApplicationDeliveryRepository(context.Context, delivery.ApplicationRepository) (delivery.ApplicationRepository, error)
	AcknowledgeApplicationDeliveryRepository(context.Context, delivery.ApplicationRepository, int64) (delivery.ApplicationRepository, error)
}
