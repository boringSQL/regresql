/*
 * Statistics restore functions for 16/17. 18+ has the relation and attribute
 * ones in core, so this creates nothing there.
 */
DO $pg_regresql$
BEGIN
	IF current_setting('server_version_num')::int BETWEEN 160000 AND 179999 THEN
		EXECUTE $fn$
			CREATE FUNCTION pg_catalog.pg_restore_relation_stats(VARIADIC kwargs "any")
			RETURNS bool
			AS 'MODULE_PATHNAME', 'regresql_pg_restore_relation_stats'
			LANGUAGE C VOLATILE PARALLEL UNSAFE
		$fn$;

		EXECUTE $fn$
			CREATE FUNCTION pg_catalog.pg_restore_attribute_stats(VARIADIC kwargs "any")
			RETURNS bool
			AS 'MODULE_PATHNAME', 'regresql_pg_restore_attribute_stats'
			LANGUAGE C VOLATILE PARALLEL UNSAFE
		$fn$;
	END IF;
END
$pg_regresql$;

/* 2.2: extended statistics, see README */
DO $pg_regresql$
BEGIN
	IF current_setting('server_version_num')::int BETWEEN 160000 AND 179999 THEN
		EXECUTE $fn$
			CREATE FUNCTION pg_catalog.pg_restore_extended_stats(VARIADIC kwargs "any")
			RETURNS bool
			AS 'MODULE_PATHNAME', 'regresql_pg_restore_extended_stats'
			LANGUAGE C VOLATILE PARALLEL UNSAFE
		$fn$;

		EXECUTE $fn$
			CREATE FUNCTION pg_catalog.pg_clear_extended_stats(
				schemaname text, relname text,
				statistics_schemaname text, statistics_name text,
				inherited bool)
			RETURNS void
			AS 'MODULE_PATHNAME', 'regresql_pg_clear_extended_stats'
			LANGUAGE C VOLATILE PARALLEL UNSAFE
		$fn$;

		/*
		 * The input functions outlive DROP EXTENSION (see below). On a
		 * re-create, adopt them again, or CREATE OR REPLACE is refused.
		 */
		IF to_regprocedure('pg_catalog.regresql_pg_ndistinct_in(cstring)') IS NOT NULL THEN
			ALTER EXTENSION pg_regresql ADD FUNCTION pg_catalog.regresql_pg_ndistinct_in(cstring);
		END IF;
		IF to_regprocedure('pg_catalog.regresql_pg_dependencies_in(cstring)') IS NOT NULL THEN
			ALTER EXTENSION pg_regresql ADD FUNCTION pg_catalog.regresql_pg_dependencies_in(cstring);
		END IF;

		/*
		 * 16/17 have the pg_ndistinct and pg_dependencies types but their
		 * input functions always fail. Point the types at the PG19 parsers.
		 * The two functions are detached from the extension below, so that
		 * DROP EXTENSION cannot leave pg_type with a dangling typinput.
		 */
		EXECUTE $fn$
			CREATE OR REPLACE FUNCTION pg_catalog.regresql_pg_ndistinct_in(cstring)
			RETURNS pg_catalog.pg_ndistinct
			AS 'MODULE_PATHNAME', 'regresql_pg_ndistinct_in'
			LANGUAGE C IMMUTABLE STRICT PARALLEL SAFE
		$fn$;

		EXECUTE $fn$
			CREATE OR REPLACE FUNCTION pg_catalog.regresql_pg_dependencies_in(cstring)
			RETURNS pg_catalog.pg_dependencies
			AS 'MODULE_PATHNAME', 'regresql_pg_dependencies_in'
			LANGUAGE C IMMUTABLE STRICT PARALLEL SAFE
		$fn$;

		UPDATE pg_catalog.pg_type
		   SET typinput = 'pg_catalog.regresql_pg_ndistinct_in(cstring)'::regprocedure
		 WHERE oid = 'pg_catalog.pg_ndistinct'::regtype;

		UPDATE pg_catalog.pg_type
		   SET typinput = 'pg_catalog.regresql_pg_dependencies_in(cstring)'::regprocedure
		 WHERE oid = 'pg_catalog.pg_dependencies'::regtype;

		ALTER EXTENSION pg_regresql DROP FUNCTION pg_catalog.regresql_pg_ndistinct_in(cstring);
		ALTER EXTENSION pg_regresql DROP FUNCTION pg_catalog.regresql_pg_dependencies_in(cstring);
	END IF;
END
$pg_regresql$;
