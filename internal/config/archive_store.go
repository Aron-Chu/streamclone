package config

import "streamclone/internal/archive"

// ArchiveBlobStoreConfig maps archive-related env fields into archive.StoreConfig.
func (c Config) ArchiveBlobStoreConfig() archive.StoreConfig {
	return archive.StoreConfig{
		Azure: archive.AzureConfig{
			StorageAccount:       c.ArchiveAzureStorageAccount,
			Container:            c.ArchiveAzureContainer,
			Prefix:               c.ArchiveAzurePrefix,
			ConnectionStringFile: c.ArchiveAzureConnectionStringFile,
		},
		R2: archive.R2Config{
			AccountID:           c.ArchiveR2AccountID,
			Bucket:              c.ArchiveR2Bucket,
			Prefix:              c.ArchiveR2Prefix,
			Endpoint:            c.ArchiveR2Endpoint,
			AccessKeyIDFile:     c.ArchiveR2AccessKeyIDFile,
			SecretAccessKeyFile: c.ArchiveR2SecretAccessKeyFile,
		},
		PrimaryProvider: c.ArchivePrimaryProvider,
		ReadThrough:     c.ArchiveReadThrough,
		DualWrite:       c.ArchiveDualWrite,
	}
}
