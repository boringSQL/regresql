/*
 * Statistics restore functions for 16/17. 18+ has them in core, so this
 * creates nothing there. Extended statistics are not restored; see README.
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
