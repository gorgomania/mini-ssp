CREATE TABLE IF NOT EXISTS dsps (
    id        SERIAL PRIMARY KEY,
    name      VARCHAR(64) UNIQUE NOT NULL,
    grpc_addr VARCHAR(255) NOT NULL,
    active    BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS freqcap_rules (
    id            SERIAL PRIMARY KEY,
    advertiser_id VARCHAR(64) UNIQUE NOT NULL,
    cap_limit     INT NOT NULL DEFAULT 3,
    window_secs   INT NOT NULL DEFAULT 3600
);

INSERT INTO dsps (name, grpc_addr) VALUES
    ('adcorp',   'dsp-adcorp:50051'),
    ('medianet', 'dsp-medianet:50051'),
    ('quickads', 'dsp-quickads:50051'),
    ('ruads',    'dsp-ruads:50051'),
    ('apacads',  'dsp-apacads:50051'),
    ('latamads', 'dsp-latamads:50051')
ON CONFLICT (name) DO NOTHING;

INSERT INTO freqcap_rules (advertiser_id, cap_limit, window_secs) VALUES
    ('adcorp',   5, 3600),
    ('medianet', 5, 3600),
    ('quickads', 3, 1800),
    ('ruads',    4, 3600),
    ('apacads',  4, 3600),
    ('latamads', 3, 3600)
ON CONFLICT (advertiser_id) DO NOTHING;
