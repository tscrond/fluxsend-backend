package service

import (
	"encoding/json"
	"testing"
)

func TestCreatePlanRequestJSONMapping(t *testing.T) {
	payload := []byte(`{"name":"zjeb","max_total_storage_bytes":111111111,"max_file_size_bytes":11111,"max_files":1,"max_files_sent_per_day":1,"max_shares_per_day":10,"max_files_workspace":1,"max_user_workspaces":5,"max_total_storage_bytes_workspace":1073741824,"max_users_workspace":10,"max_workspace_folders":20,"max_private_api_keys":10,"max_workspace_api_keys":10}`)

	var req CreatePlanRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if req.Name != "zjeb" || req.MaxTotalStorageBytes != 111111111 || req.MaxFileSizeBytes != 11111 || req.MaxFiles != 1 || req.MaxFilesSentPerDay != 1 || req.MaxSharesPerDay != 10 || req.MaxFilesWorkspace != 1 || req.MaxUserWorkspaces != 5 || req.MaxTotalStorageBytesWorkspace != 1073741824 || req.MaxUsersWorkspace != 10 || req.MaxWorkspaceFolders != 20 || req.MaxPrivateAPIKeys != 10 || req.MaxWorkspaceAPIKeys != 10 {
		t.Fatalf("decoded request mismatch: %#v", req)
	}
}
