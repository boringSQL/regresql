-- Compare restored rows with the ANALYZE'd originals.
\set ON_ERROR_STOP 1
CREATE TABLE rt.after AS
SELECT s.stxname,
       d.stxdndistinct::bytea AS nd, d.stxddependencies::bytea AS dep,
       d.stxdmcv::bytea AS mcv, d.stxdexpr::text AS expr,
       d.stxddependencies::text AS dep_text
  FROM pg_statistic_ext s JOIN pg_statistic_ext_data d ON d.stxoid = s.oid
 WHERE s.stxnamespace = 'rt'::regnamespace AND NOT d.stxdinherit;

DO $$
DECLARE
	r record;
	bad int := 0;
BEGIN
	-- byte equality per kind; dependencies of s5 carry fractional degrees that the
	-- 16/17 text output truncates to 6 digits, so those are compared as text
	FOR r IN SELECT o.stxname, o.nd = a.nd AS nd_eq, o.dep = a.dep AS dep_eq,
	                o.mcv = a.mcv AS mcv_eq, o.expr = a.expr AS expr_eq,
	                o.dep_text = a.dep_text AS dep_text_eq,
	                o.nd IS NULL AS no_nd, o.dep IS NULL AS no_dep,
	                o.mcv IS NULL AS no_mcv, o.expr IS NULL AS no_expr
	           FROM rt.orig o JOIN rt.after a USING (stxname) ORDER BY 1 LOOP
		IF NOT r.no_nd THEN
			IF r.nd_eq THEN RAISE NOTICE 'PASS % ndistinct bytes identical', r.stxname;
			ELSE RAISE WARNING 'FAIL % ndistinct bytes differ', r.stxname; bad := bad + 1; END IF;
		END IF;
		IF NOT r.no_dep THEN
			IF r.dep_eq THEN RAISE NOTICE 'PASS % dependencies bytes identical', r.stxname;
			ELSIF r.dep_text_eq THEN RAISE NOTICE 'NOTE % dependencies bytes differ, text identical (degree precision)', r.stxname;
			ELSE RAISE WARNING 'FAIL % dependencies differ', r.stxname; bad := bad + 1; END IF;
		END IF;
		IF NOT r.no_mcv THEN
			IF r.mcv_eq THEN RAISE NOTICE 'PASS % mcv bytes identical', r.stxname;
			ELSE RAISE WARNING 'FAIL % mcv bytes differ', r.stxname; bad := bad + 1; END IF;
		END IF;
		IF NOT r.no_expr THEN
			IF r.expr_eq THEN RAISE NOTICE 'PASS % exprs identical (text of stxdexpr)', r.stxname;
			ELSE RAISE WARNING 'FAIL % exprs differ', r.stxname; bad := bad + 1; END IF;
		END IF;
	END LOOP;
	IF bad > 0 THEN RAISE EXCEPTION 'FAIL compare: % mismatches', bad; END IF;
END $$;

-- kinds present in the original must be present after (and nothing extra)
DO $$
DECLARE n int;
BEGIN
	SELECT count(*) INTO n FROM rt.orig o JOIN rt.after a USING (stxname)
	 WHERE (o.nd IS NULL) <> (a.nd IS NULL) OR (o.dep IS NULL) <> (a.dep IS NULL)
	    OR (o.mcv IS NULL) <> (a.mcv IS NULL) OR (o.expr IS NULL) <> (a.expr IS NULL);
	IF n > 0 THEN RAISE EXCEPTION 'FAIL kind presence differs for % objects', n; END IF;
	RAISE NOTICE 'PASS same kinds present after restore';
END $$;

-- planner estimates
CREATE TABLE rt.est_after AS SELECT n, rt.est(q) AS rows FROM rt.queries;
DO $$
DECLARE r record; bad int := 0;
BEGIN
	FOR r IN SELECT q.n, q.q, o.rows AS orig, c.rows AS cleared, a.rows AS after
	           FROM rt.queries q JOIN rt.est_orig o USING (n)
	           JOIN rt.est_cleared c USING (n) JOIN rt.est_after a USING (n) ORDER BY 1 LOOP
		RAISE NOTICE '% orig=% cleared=% restored=%  %', r.n, r.orig, r.cleared, r.after, r.q;
		IF r.orig <> r.after THEN bad := bad + 1; END IF;
	END LOOP;
	IF bad > 0 THEN RAISE EXCEPTION 'FAIL estimates: % differ', bad; END IF;
	RAISE NOTICE 'PASS all % estimates identical to the ANALYZE originals', (SELECT count(*) FROM rt.queries);
END $$;
