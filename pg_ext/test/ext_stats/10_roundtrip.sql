-- Clear every extended statistics row, then restore it from the catalog views,
-- converting the 16/17 text formats into the PG19 ones. Expect no WARNING.
\set ON_ERROR_STOP 1
-- snapshot the ANALYZE output first, it is gone once the statistics are cleared
CREATE TABLE rt.src_ext AS
	SELECT * FROM pg_stats_ext WHERE statistics_schemaname = 'rt' AND NOT inherited;
CREATE TABLE rt.src_exprs AS
	SELECT statistics_name, expr, null_frac, avg_width, n_distinct,
	       most_common_vals::text AS most_common_vals, most_common_freqs,
	       histogram_bounds::text AS histogram_bounds, correlation,
	       most_common_elems::text AS most_common_elems, most_common_elem_freqs,
	       elem_count_histogram
	  FROM pg_stats_ext_exprs WHERE statistics_schemaname = 'rt' AND NOT inherited;

CREATE OR REPLACE FUNCTION rt.nd_to_pg19(t text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
	SELECT jsonb_agg(jsonb_build_object(
	           'attributes', to_jsonb(string_to_array(key, ', ')::int[]),
	           'ndistinct', value::int))::text
	  FROM jsonb_each_text(t::jsonb) $$;

CREATE OR REPLACE FUNCTION rt.dep_to_pg19(t text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
	SELECT jsonb_agg(jsonb_build_object(
	           'attributes', to_jsonb(string_to_array(split_part(key, ' => ', 1), ', ')::int[]),
	           'dependency', split_part(key, ' => ', 2)::int,
	           'degree', value::numeric))::text
	  FROM jsonb_each_text(t::jsonb) $$;

CREATE OR REPLACE FUNCTION rt.exprs_json(sname text) RETURNS jsonb LANGUAGE sql AS $$
	SELECT jsonb_agg(jsonb_build_object(
	           'null_frac', null_frac::text, 'avg_width', avg_width::text,
	           'n_distinct', n_distinct::text,
	           'most_common_vals', most_common_vals::text,
	           'most_common_freqs', most_common_freqs::text,
	           'histogram_bounds', histogram_bounds::text,
	           'correlation', correlation::text,
	           'most_common_elems', most_common_elems::text,
	           'most_common_elem_freqs', most_common_elem_freqs::text,
	           'elem_count_histogram', elem_count_histogram::text))
	  FROM rt.src_exprs
	 WHERE statistics_name = sname $$;

-- via_text: pass n_distinct/dependencies as plain text instead of the typed cast
CREATE OR REPLACE FUNCTION rt.restore_one(sname text, via_text bool DEFAULT false) RETURNS bool
LANGUAGE plpgsql AS $$
DECLARE
	e rt.src_ext%ROWTYPE;
	ex jsonb;
	nd text; dp text;
	ok bool;
BEGIN
	SELECT * INTO e FROM rt.src_ext WHERE statistics_name = sname;
	ex := rt.exprs_json(sname);
	nd := CASE WHEN e.n_distinct IS NULL THEN NULL ELSE rt.nd_to_pg19(e.n_distinct::text) END;
	dp := CASE WHEN e.dependencies IS NULL THEN NULL ELSE rt.dep_to_pg19(e.dependencies::text) END;
	IF via_text THEN
		SELECT pg_restore_extended_stats(
			'schemaname', 'rt', 'relname', 't',
			'statistics_schemaname', 'rt', 'statistics_name', sname,
			'inherited', false,
			'n_distinct', nd::text, 'dependencies', dp::text,
			'most_common_vals', e.most_common_vals,
			'most_common_freqs', e.most_common_freqs,
			'most_common_base_freqs', e.most_common_base_freqs,
			'exprs', ex) INTO ok;
	ELSE
		EXECUTE 'SELECT pg_restore_extended_stats(
			''schemaname'', ''rt'', ''relname'', ''t'',
			''statistics_schemaname'', ''rt'', ''statistics_name'', $1,
			''inherited'', false,
			''n_distinct'', $2::pg_ndistinct, ''dependencies'', $3::pg_dependencies,
			''most_common_vals'', $4::text[],
			''most_common_freqs'', $5::float8[],
			''most_common_base_freqs'', $6::float8[],
			''exprs'', $7::jsonb)'
		INTO ok USING sname, nd, dp, e.most_common_vals, e.most_common_freqs,
		              e.most_common_base_freqs, ex;
	END IF;
	RETURN ok;
END $$;

-- clear
DO $$
DECLARE r record; n int;
BEGIN
	FOR r IN SELECT stxname FROM rt.orig LOOP
		PERFORM pg_clear_extended_stats('rt', 't', 'rt', r.stxname, false);
	END LOOP;
	SELECT count(*) INTO n FROM pg_statistic_ext_data d JOIN pg_statistic_ext s ON s.oid = d.stxoid
	 WHERE s.stxnamespace = 'rt'::regnamespace;
	IF n <> 0 THEN RAISE EXCEPTION 'FAIL clear: % rows left', n; END IF;
	RAISE NOTICE 'PASS clear: pg_statistic_ext_data empty';
END $$;

CREATE TABLE rt.est_cleared AS SELECT n, rt.est(q) AS rows FROM rt.queries;
DO $$
DECLARE n int;
BEGIN
	SELECT count(*) INTO n FROM rt.est_cleared c JOIN rt.est_orig o USING (n) WHERE c.rows <> o.rows;
	IF n = 0 THEN RAISE EXCEPTION 'FAIL sensitivity: clearing changed no estimate'; END IF;
	RAISE NOTICE 'PASS sensitivity: % of % estimates change without the statistics',
		n, (SELECT count(*) FROM rt.queries);
END $$;

-- restore: s1..s3 typed, s4..s5 as text
DO $$
DECLARE r record; ok bool;
BEGIN
	FOR r IN SELECT stxname FROM rt.orig ORDER BY 1 LOOP
		ok := rt.restore_one(r.stxname, r.stxname IN ('s4', 's5'));
		IF ok IS DISTINCT FROM true THEN
			RAISE EXCEPTION 'FAIL restore %: returned %', r.stxname, ok;
		END IF;
		RAISE NOTICE 'PASS restore %: true (%)', r.stxname,
			CASE WHEN r.stxname IN ('s4', 's5') THEN 'text args' ELSE 'typed args' END;
	END LOOP;
END $$;
