MODULE_big = pg_regresql
OBJS = pg_regresql.o
EXTENSION = pg_regresql
DATA = pg_regresql--2.0.sql
VERSION = 2.0
DISTNAME = $(EXTENSION)-$(VERSION)
DIST_FILES = META.json Makefile $(EXTENSION).control $(DATA) $(EXTENSION).c README.md LICENSE

dist:
	rm -rf /tmp/$(DISTNAME) $(DISTNAME).zip
	mkdir /tmp/$(DISTNAME)
	cp $(DIST_FILES) /tmp/$(DISTNAME)/
	cd /tmp && zip -r $(CURDIR)/$(DISTNAME).zip $(DISTNAME)/
	rm -rf /tmp/$(DISTNAME)

clean:
	rm -f $(OBJS) $(MODULE_big).so $(MODULE_big).dylib $(MODULE_big).bc

ifeq ($(filter clean dist,$(MAKECMDGOALS)),)
ifndef PG_SOURCE
PG_CONFIG ?= pg_config
PGXS := $(shell $(PG_CONFIG) --pgxs)
include $(PGXS)
else
subdir = contrib/pg_regresql
top_builddir = $(PG_SOURCE)
top_srcdir = $(PG_SOURCE)
include $(PG_SOURCE)/src/Makefile.global
include $(PG_SOURCE)/contrib/contrib-global.mk
endif
endif
