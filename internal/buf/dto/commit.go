package dto

import (
	"encoding/hex"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	generalutils "github.com/alipourhabibi/Hades/utils/general"
	"github.com/google/uuid"
)

// ToCommitPB converts an internal Commit to the buf.build wire type.
// Digest.Value is expected to be a hex string (as stored in the DB); it is
// decoded to raw bytes here. If decoding fails the raw bytes are used as-is.
func ToCommitPB(in *registryv1.Commit) *modulev1.Commit {
	tId, _ := uuid.Parse(in.Id)

	var digestValue []byte
	if len(in.Digest.GetValue()) > 0 {
		decoded, err := hex.DecodeString(string(in.Digest.Value))
		if err == nil {
			digestValue = decoded
		} else {
			digestValue = in.Digest.Value
		}
	}

	return &modulev1.Commit{
		Id:         generalutils.ToDashless(tId),
		CreateTime: in.CreateTime,
		OwnerId:    in.OwnerId,
		ModuleId:   in.ModuleId,
		Digest: &modulev1.Digest{
			Type:  modulev1.DigestType(in.Digest.Type),
			Value: digestValue,
		},
		CreatedByUserId:  in.CreatedByUserId,
		SourceControlUrl: in.SourceControlUrl,
	}
}
