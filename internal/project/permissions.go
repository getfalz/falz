package project

import "io/fs"

const (
	// OutputDirPerm is used for generated output directories such as dist/.
	OutputDirPerm fs.FileMode = 0o755

	// OutputFilePerm is used for generated build artifacts and public metadata.
	OutputFilePerm fs.FileMode = 0o644

	// CacheDirPerm is used for private CAS shard directories.
	CacheDirPerm fs.FileMode = 0o700

	// CacheFilePerm is used for private CAS objects.
	CacheFilePerm fs.FileMode = 0o600
)
