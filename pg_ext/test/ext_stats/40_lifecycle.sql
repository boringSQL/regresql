-- DROP/CREATE EXTENSION keeps the casts working, 2.1 -> 2.2 update works.
\set ON_ERROR_STOP 1
CREATE OR REPLACE FUNCTION rt.cast_ok() RETURNS bool LANGUAGE plpgsql AS $$
DECLARE a pg_ndistinct; b pg_dependencies;
BEGIN
	a := '[{"attributes" : [11,14], "ndistinct" : 24268}]'::pg_ndistinct;
	b := '[{"attributes" : [11], "dependency" : 14, "degree" : 0.998233}]'::pg_dependencies;
	RETURN a IS NOT NULL AND b IS NOT NULL AND a::bytea <> b::bytea;
END $$;

DO $$ BEGIN
	IF NOT rt.cast_ok() THEN RAISE EXCEPTION 'FAIL casts before drop'; END IF;
	RAISE NOTICE 'PASS casts work with extension 2.2';
	IF (SELECT count(*) FROM pg_depend d JOIN pg_extension e ON e.oid = d.refobjid
	     WHERE d.classid = 'pg_proc'::regclass AND d.deptype = 'e' AND e.extname = 'pg_regresql'
	       AND d.objid::regproc::text LIKE '%regresql_pg_%_in') <> 0 THEN
		RAISE EXCEPTION 'FAIL input functions still members of the extension';
	END IF;
	RAISE NOTICE 'PASS input functions are detached from the extension';
END $$;

DROP EXTENSION pg_regresql;
DO $$ BEGIN
	IF NOT rt.cast_ok() THEN RAISE EXCEPTION 'FAIL casts after DROP EXTENSION'; END IF;
	IF to_regprocedure('pg_restore_extended_stats("any")') IS NOT NULL THEN
		RAISE EXCEPTION 'FAIL restore function survived DROP EXTENSION';
	END IF;
	RAISE NOTICE 'PASS casts still work after DROP EXTENSION, restore functions gone';
END $$;

CREATE EXTENSION pg_regresql;
DO $$ BEGIN
	IF NOT rt.cast_ok() THEN RAISE EXCEPTION 'FAIL casts after re-CREATE'; END IF;
	IF to_regprocedure('pg_restore_extended_stats("any")') IS NULL THEN
		RAISE EXCEPTION 'FAIL restore function missing after re-CREATE';
	END IF;
	RAISE NOTICE 'PASS DROP + CREATE EXTENSION works, casts work';
END $$;

-- upgrade path from 2.1
DROP EXTENSION pg_regresql;
CREATE EXTENSION pg_regresql VERSION '2.1';
DO $$ BEGIN
	IF to_regprocedure('pg_restore_extended_stats("any")') IS NOT NULL THEN
		RAISE EXCEPTION 'FAIL 2.1 has the extended function'; END IF;
	RAISE NOTICE 'PASS 2.1 installed without extended stats functions';
END $$;
ALTER EXTENSION pg_regresql UPDATE;
DO $$ BEGIN
	IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_regresql') <> '2.2' THEN
		RAISE EXCEPTION 'FAIL version not 2.2'; END IF;
	IF NOT rt.cast_ok() THEN RAISE EXCEPTION 'FAIL casts after update'; END IF;
	IF NOT (SELECT pg_restore_extended_stats('schemaname', 'rt', 'relname', 't',
	        'statistics_schemaname', 'rt', 'statistics_name', 's1', 'inherited', false,
	        'n_distinct', '[{"attributes" : [11,14], "ndistinct" : 100}, {"attributes" : [11,13], "ndistinct" : 100}, {"attributes" : [13,14], "ndistinct" : 10}, {"attributes" : [11,13,14], "ndistinct" : 100}]'::pg_ndistinct)) THEN
		RAISE EXCEPTION 'FAIL restore after update'; END IF;
	RAISE NOTICE 'PASS ALTER EXTENSION UPDATE 2.1 -> 2.2 works, restore works';
END $$;
-- leave the old extension state in a known place
DROP EXTENSION pg_regresql;
CREATE EXTENSION pg_regresql;
SELECT 'PASS lifecycle done';
