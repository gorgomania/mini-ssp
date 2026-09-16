CREATE TABLE IF NOT EXISTS default.auction_events
(
    ts             DateTime,
    user_id        String,
    geo            LowCardinality(String),
    format         LowCardinality(String),
    advertiser_id  LowCardinality(String),
    clearing_price Float64
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(ts)
ORDER BY (geo, format, advertiser_id, ts);
