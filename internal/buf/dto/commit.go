package dto

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	generalutils "github.com/alipourhabibi/Hades/utils/general"
	"github.com/google/uuid"
)

func ToCommitPB(in *registryv1.Commit) *modulev1.Commit {
	tId, _ := uuid.Parse(in.Id)
	return &modulev1.Commit{
		Id:         generalutils.ToDashless(tId),
		CreateTime: in.CreateTime,
		OwnerId:    in.OwnerId,
		ModuleId:   in.ModuleId,
		Digest: &modulev1.Digest{
			Type:  modulev1.DigestType(in.Digest.Type),
			Value: []byte(in.Digest.Value),
		},
		CreatedByUserId:  in.CreatedByUserId,
		SourceControlUrl: in.SourceControlUrl,
	}
}
