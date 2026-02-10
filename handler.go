package main

import (
	"context"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
)

// ImageModerationServiceImpl implements the last service interface defined in the IDL.
type ImageModerationServiceImpl struct{}

// Moderate implements the ImageModerationServiceImpl interface.
func (s *ImageModerationServiceImpl) Moderate(ctx context.Context, req *safeflow.ImageModerationRequest) (resp *safeflow.ImageModerationResponse, err error) {
	// TODO: Your code here...
	return
}
