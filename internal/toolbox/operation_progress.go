package toolbox

import (
	"context"
	"time"
)

// OperationProgress is the latest stage of one item, separate from completed
// batch items. Consumers should replace the value rather than accumulate it.
type OperationProgress struct {
	Tool, Source, Phase   string
	ItemNumber, ItemTotal int
	Percent               float64
	PercentKnown          bool
	FilesDone, FilesTotal int
	BytesDone, BytesTotal int64
	Started, PhaseStarted time.Time
}
type operationProgressKey struct{}
type copyProgressKey struct{}

func WithOperationProgress(ctx context.Context, notify func(OperationProgress)) context.Context {
	return context.WithValue(ctx, operationProgressKey{}, notify)
}
func reportOperationProgress(ctx context.Context, p OperationProgress) {
	if notify, ok := ctx.Value(operationProgressKey{}).(func(OperationProgress)); ok && notify != nil {
		notify(p)
	}
}
