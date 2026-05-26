CREATE TABLE refdata_fetch_log (
    data_type  VARCHAR(32) PRIMARY KEY,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
