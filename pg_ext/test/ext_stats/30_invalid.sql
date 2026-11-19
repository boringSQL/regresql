-- Bad input must produce a WARNING and false, never a crash.
\set ON_ERROR_STOP 1
CREATE OR REPLACE FUNCTION rt.try(label text, expect bool, sql text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE ok bool;
BEGIN
	EXECUTE replace(sql, '@BASE@', '''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''rt'', ''statistics_name'', ''s1'', ''inherited'', false') INTO ok;
	IF ok IS DISTINCT FROM expect THEN
		RAISE EXCEPTION 'FAIL %: returned %, expected %', label, ok, expect;
	END IF;
	RAISE NOTICE 'PASS %: %', label, ok;
EXCEPTION WHEN OTHERS THEN
	IF expect IS NULL THEN RAISE NOTICE 'PASS %: error "%"', label, SQLERRM;
	ELSE RAISE; END IF;
END $$;


-- attnum 99 is not in s1 (11,13,14)
SELECT rt.try('ndistinct wrong attnum', false, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', ''[{"attributes" : [11,99], "ndistinct" : 5}]''::pg_ndistinct)');
SELECT rt.try('dependencies wrong attnum', false, 'SELECT pg_restore_extended_stats(@BASE@, ''dependencies'', ''[{"attributes" : [11], "dependency" : 12, "degree" : 0.5}]''::pg_dependencies)');
-- malformed
SELECT rt.try('ndistinct malformed json (cast)', NULL, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', ''[{"attributes" : [11,14], "ndistinct" ''::pg_ndistinct)');
SELECT rt.try('ndistinct malformed json (text)', false, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', ''[{"attributes" : [11,14], "ndistinct" ''::text)');
SELECT rt.try('dependencies malformed (text)', false, 'SELECT pg_restore_extended_stats(@BASE@, ''dependencies'', ''[{"attributes" : [11], "degree" : 1}]''::text)');
SELECT rt.try('ndistinct old 16 format (text)', false, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', ''{"11, 14": 5}''::text)');
SELECT rt.try('ndistinct empty (text)', false, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', ''''::text)');
SELECT rt.try('ndistinct attributes unordered (text)', false, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', ''[{"attributes" : [14,11], "ndistinct" : 5}]''::text)');
-- wrong kwarg type, unknown kwarg
SELECT rt.try('n_distinct given as int', false, 'SELECT pg_restore_extended_stats(@BASE@, ''n_distinct'', 5)');
SELECT rt.try('unknown kwarg', false, 'SELECT pg_restore_extended_stats(@BASE@, ''bogus'', 5)');
-- missing object / wrong relation
SELECT rt.try('missing stats object', false, 'SELECT pg_restore_extended_stats(''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''rt'', ''statistics_name'', ''nope'', ''inherited'', false)');
SELECT rt.try('missing schema', false, 'SELECT pg_restore_extended_stats(''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''nope'', ''statistics_name'', ''s1'', ''inherited'', false)');
-- kind the object does not have (s3 is mcv only)
SELECT rt.try('ndistinct on mcv-only object', false, 'SELECT pg_restore_extended_stats(''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''rt'', ''statistics_name'', ''s3'', ''inherited'', false, ''n_distinct'', ''[{"attributes" : [11,12], "ndistinct" : 5}]''::pg_ndistinct)');
-- mcv: bad shapes
SELECT rt.try('mcv wrong width', false, 'SELECT pg_restore_extended_stats(@BASE@, ''most_common_vals'', ''{{1,2}}''::text[], ''most_common_freqs'', ''{0.5}''::float8[], ''most_common_base_freqs'', ''{0.5}''::float8[])');
SELECT rt.try('mcv bad value', false, 'SELECT pg_restore_extended_stats(@BASE@, ''most_common_vals'', ''{{x,1,c1}}''::text[], ''most_common_freqs'', ''{0.5}''::float8[], ''most_common_base_freqs'', ''{0.5}''::float8[])');
SELECT rt.try('mcv partial', false, 'SELECT pg_restore_extended_stats(@BASE@, ''most_common_vals'', ''{{1,1,c1}}''::text[])');
-- exprs: wrong count, bad key, wrong shape
SELECT rt.try('exprs on object without expressions', false, 'SELECT pg_restore_extended_stats(@BASE@, ''exprs'', ''[{"null_frac": "0"}]''::jsonb)');
SELECT rt.try('exprs wrong count', false, 'SELECT pg_restore_extended_stats(''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''rt'', ''statistics_name'', ''s2'', ''inherited'', false, ''exprs'', ''[]''::jsonb)');
SELECT rt.try('exprs bad key', false, 'SELECT pg_restore_extended_stats(''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''rt'', ''statistics_name'', ''s2'', ''inherited'', false, ''exprs'', ''[{"bogus": "1"}]''::jsonb)');
SELECT rt.try('exprs bad float', false, 'SELECT pg_restore_extended_stats(''schemaname'', ''rt'', ''relname'', ''t'', ''statistics_schemaname'', ''rt'', ''statistics_name'', ''s2'', ''inherited'', false, ''exprs'', ''[{"null_frac": "abc"}]''::jsonb)');
-- cast level behaviour
SELECT rt.try('cast good ndistinct', true, $q$SELECT '[{"attributes" : [11,14], "ndistinct" : 5}]'::pg_ndistinct IS NOT NULL$q$);
SELECT rt.try('soft input: pg_input_is_valid good', true, $q$SELECT pg_input_is_valid('[{"attributes" : [11,14], "ndistinct" : 5}]', 'pg_ndistinct')$q$);
SELECT rt.try('soft input: pg_input_is_valid bad', false, $q$SELECT pg_input_is_valid('[{"attributes" : [11,14]', 'pg_ndistinct')$q$);
SELECT rt.try('soft input: dependencies valid', true, $q$SELECT pg_input_is_valid('[{"attributes" : [11], "dependency" : 14, "degree" : 0.998233}]', 'pg_dependencies')$q$);
SELECT rt.try('soft input: dependencies bad', false, $q$SELECT pg_input_is_valid('[{"attributes" : [], "dependency" : 14, "degree" : 0.9}]', 'pg_dependencies')$q$);

-- clear with unknown object: warning, no error
SELECT pg_clear_extended_stats('rt', 't', 'rt', 'nope', false);
SELECT pg_clear_extended_stats('rt', 't', 'nope', 's1', false);

-- the payload shapes of the realistic case: attnums 11 and 14 in a 15 column table
SELECT rt.try('realistic [11,14] ndistinct + [11]->14 dependency', true, $q$SELECT pg_restore_extended_stats(
	'schemaname', 'rt', 'relname', 't', 'statistics_schemaname', 'rt', 'statistics_name', 's1', 'inherited', false,
	'n_distinct', '[{"attributes" : [11,14], "ndistinct" : 100}, {"attributes" : [11,13], "ndistinct" : 100}, {"attributes" : [13,14], "ndistinct" : 10}, {"attributes" : [11,13,14], "ndistinct" : 100}]'::pg_ndistinct,
	'dependencies', '[{"attributes" : [11], "dependency" : 14, "degree" : 0.998233}]'::pg_dependencies)$q$);
SELECT 'restored values: ' || stxdndistinct::text || ' / ' || stxddependencies::text
  FROM pg_statistic_ext_data d JOIN pg_statistic_ext s ON s.oid = d.stxoid WHERE s.stxname = 's1';
