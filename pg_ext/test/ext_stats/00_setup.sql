-- Fixture: a 15 column table so attnums like 11 and 14 occur, correlated data,
-- five statistics objects covering every kind, ANALYZE'd, originals saved in rt.*.
\set ON_ERROR_STOP 1
SET client_min_messages = warning;
DROP SCHEMA IF EXISTS rt CASCADE;
DROP EXTENSION IF EXISTS pg_regresql;
CREATE EXTENSION pg_regresql;
CREATE SCHEMA rt;

SELECT setseed(0.42);
CREATE TABLE rt.t (
	id int, f1 int, f2 int, f3 int, f4 int, f5 int, f6 int, f7 int, f8 int, f9 int,
	a int,        -- attnum 11
	noise int,    -- 12
	c text,       -- 13
	b int,        -- 14
	d bigint      -- 15
);
INSERT INTO rt.t
SELECT i, i%3, i%5, i%7, i%11, i%13, i%17, i%19, i%23, i%29,
       i%100,
       CASE WHEN random() < 0.05 THEN (random()*9)::int ELSE (i%100)%10 END,
       'c' || ((i%100)%7),
       (i%100)%10,
       CASE WHEN i%97 = 0 THEN NULL ELSE (i%100)*3 + (random()*2)::int END
FROM generate_series(1, 50000) i;

CREATE STATISTICS rt.s1 (ndistinct, dependencies, mcv) ON a, b, c FROM rt.t;
CREATE STATISTICS rt.s2 ON (a+b), c FROM rt.t;
CREATE STATISTICS rt.s3 (mcv) ON noise, d FROM rt.t;
CREATE STATISTICS rt.s4 ON (a*2), (b+1) FROM rt.t;
CREATE STATISTICS rt.s5 (ndistinct, dependencies) ON a, noise FROM rt.t;
ANALYZE rt.t;

-- exact (lossless) vs lossy: dependency degrees are printed with %f, so only
-- objects whose degrees are 0/1 can round trip byte for byte through text.
CREATE TABLE rt.orig AS
SELECT s.stxname,
       d.stxdndistinct::bytea  AS nd,
       d.stxddependencies::bytea AS dep,
       d.stxdmcv::bytea        AS mcv,
       d.stxdexpr::text        AS expr,
       d.stxddependencies::text AS dep_text
  FROM pg_statistic_ext s JOIN pg_statistic_ext_data d ON d.stxoid = s.oid
 WHERE s.stxnamespace = 'rt'::regnamespace AND NOT d.stxdinherit;

CREATE FUNCTION rt.est(q text) RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE j json;
BEGIN
	EXECUTE 'EXPLAIN (FORMAT JSON) ' || q INTO j;
	RETURN (j->0->'Plan'->>'Plan Rows')::bigint;
END $$;

CREATE TABLE rt.queries (n serial PRIMARY KEY, q text);
INSERT INTO rt.queries (q) VALUES
 ('SELECT * FROM rt.t WHERE a = 5 AND b = 5'),
 ('SELECT * FROM rt.t WHERE a = 5 AND b = 7'),
 ('SELECT * FROM rt.t WHERE a = 5 AND b = 5 AND c = ''c5'''),
 ('SELECT * FROM rt.t WHERE a = 12 AND c = ''c5'''),
 ('SELECT 1 FROM rt.t GROUP BY a, b'),
 ('SELECT 1 FROM rt.t GROUP BY a, b, c'),
 ('SELECT * FROM rt.t WHERE a = 3 AND noise = 3'),
 ('SELECT * FROM rt.t WHERE a = 3 AND noise = 6'),
 ('SELECT 1 FROM rt.t GROUP BY a, noise'),
 ('SELECT * FROM rt.t WHERE noise = 3 AND d = 10'),
 ('SELECT * FROM rt.t WHERE noise = 3 AND d IS NULL'),
 ('SELECT * FROM rt.t WHERE (a+b) = 7 AND c = ''c1'''),
 ('SELECT * FROM rt.t WHERE (a+b) = 7'),
 ('SELECT 1 FROM rt.t GROUP BY (a+b), c'),
 ('SELECT * FROM rt.t WHERE (a*2) = 10 AND (b+1) = 6'),
 ('SELECT 1 FROM rt.t GROUP BY (a*2), (b+1)');
CREATE TABLE rt.est_orig AS SELECT n, rt.est(q) AS rows FROM rt.queries;

SELECT 'PASS setup: ' || count(*) || ' stats objects, ' || (SELECT count(*) FROM rt.queries) || ' queries'
  FROM rt.orig;
