module github.com/aibank/platform/tools/outbox-cleanup

go 1.22

require (
	github.com/aibank/platform/packages/outbox v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.10.9
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/segmentio/kafka-go v0.4.47 // indirect
)

replace github.com/aibank/platform/packages/outbox => ../../packages/outbox
