CREATE TABLE file_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    owner_id UUID NOT NULL,

    storage_backend TEXT NOT NULL,

    -- Provider-specific upload/session identifier
    storage_upload_id TEXT,

    storage_mapping UUID NOT NULL DEFAULT gen_random_uuid(),

    file_name TEXT NOT NULL,
    file_type TEXT,
    expected_size BIGINT NOT NULL,

    uploaded_size BIGINT NOT NULL DEFAULT 0,

    status TEXT NOT NULL DEFAULT 'uploading',

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),

    CONSTRAINT file_uploads_owner_id_fkey
        FOREIGN KEY (owner_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT file_uploads_status_check
        CHECK (status IN ('uploading', 'completed', 'aborted', 'failed'))
);


CREATE UNIQUE INDEX file_uploads_storage_mapping_unique
    ON file_uploads(storage_mapping);


CREATE INDEX file_uploads_owner_id_idx
    ON file_uploads(owner_id);


CREATE INDEX file_uploads_status_idx
    ON file_uploads(status);


CREATE TABLE file_upload_parts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    upload_id UUID NOT NULL,

    part_number INTEGER NOT NULL,

    storage_metadata JSONB NOT NULL DEFAULT '{}',

    size BIGINT NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),

    CONSTRAINT file_upload_parts_upload_id_fkey
        FOREIGN KEY (upload_id)
        REFERENCES file_uploads(id)
        ON DELETE CASCADE,

    CONSTRAINT file_upload_parts_unique
        UNIQUE(upload_id, part_number)
);


CREATE INDEX file_upload_parts_upload_id_idx
    ON file_upload_parts(upload_id);