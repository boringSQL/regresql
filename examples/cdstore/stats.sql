--
-- PostgreSQL database dump
--

\restrict OAubpvAxxekVROwIqsQpMT8FI89OvgJRJiI01BBLeC8hikpUN7TuV3dVFAgdEOp

-- Dumped from database version 18.3 (Debian 18.3-1.pgdg12+1)
-- Dumped by pg_dump version 18.3 (Debian 18.3-1.pgdg12+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Statistics for Name: album; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album',
	'relpages', '1'::integer,
	'reltuples', '23'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album',
	'attname', 'album_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{1,4,7,10,30,31,32,35,36,37,40,50,51,70,80,85,86,95,96,193,194,229,230}'::text,
	'correlation', '0.9130435'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album',
	'attname', 'artist_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-0.5217391'::real,
	'most_common_vals', '{1,22,50,84,90,120,127,152}'::text,
	'most_common_freqs', '{0.13043478,0.13043478,0.13043478,0.08695652,0.08695652,0.08695652,0.08695652,0.08695652}'::real[],
	'histogram_bounds', '{8,58,110,118}'::text,
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album',
	'attname', 'created_at',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '8'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{"2025-11-01 01:00:00+01","2025-11-02 01:00:00+01","2025-11-03 01:00:00+01","2025-11-04 01:00:00+01","2025-11-05 01:00:00+01","2025-11-06 01:00:00+01","2025-11-07 01:00:00+01","2025-11-08 01:00:00+01","2025-11-09 01:00:00+01","2025-11-10 01:00:00+01","2025-11-11 01:00:00+01","2025-11-12 01:00:00+01","2025-11-13 01:00:00+01","2025-11-14 01:00:00+01","2025-11-15 01:00:00+01","2025-11-16 01:00:00+01","2025-11-17 01:00:00+01","2025-11-18 01:00:00+01","2025-11-19 01:00:00+01","2025-11-20 01:00:00+01","2025-11-21 01:00:00+01","2025-11-22 01:00:00+01","2025-11-23 01:00:00+01"}'::text,
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album',
	'attname', 'title',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '17'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{"...And Justice for All",Audioslave,"Blood Sugar Sex Magik",Californication,"For Those About To Rock We Salute You","Highway to Hell","Houses of the Holy","Kid A","Led Zeppelin IV","Let There Be Rock","Machine Head","Master of Puppets",Nevermind,"OK Computer","Physical Graffiti",Powerslave,"Ride the Lightning",Ten,"The Colour and the Shape","The Dark Side of the Moon","The Number of the Beast","Wasting Light","Wish You Were Here"}'::text,
	'correlation', '0.30731225'::real
);


--
-- Statistics for Name: artist; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'artist',
	'relpages', '1'::integer,
	'reltuples', '20'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'artist',
	'attname', 'artist_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{1,2,8,15,22,50,58,76,82,84,90,92,100,110,118,120,127,140,152,200}'::text,
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'artist',
	'attname', 'name',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '13'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{Accept,AC/DC,Audioslave,"Buddy Guy","Creedence Clearwater Revival","Deep Purple","Faith No More","Foo Fighters","Iron Maiden",Jamiroquai,"Led Zeppelin","Lenny Kravitz",Metallica,Nirvana,"Pearl Jam","Pink Floyd","Queens of the Stone Age",Radiohead,"Red Hot Chili Peppers","The Black Keys"}'::text,
	'correlation', '0.9007519'::real
);


--
-- Statistics for Name: genre; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'genre',
	'relpages', '1'::integer,
	'reltuples', '10'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'genre',
	'attname', 'genre_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{1,2,3,4,5,6,7,8,9,10}'::text,
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'genre',
	'attname', 'name',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '7'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{"Alternative & Punk",Blues,Classical,Jazz,Latin,Metal,Pop,Reggae,Rock,Soundtrack}'::text,
	'correlation', '0.3212121'::real
);


--
-- Statistics for Name: media_type; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'media_type',
	'relpages', '1'::integer,
	'reltuples', '5'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'media_type',
	'attname', 'media_type_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{1,2,3,4,5}'::text,
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'media_type',
	'attname', 'name',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '21'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{"AAC audio file","MPEG audio file","Protected AAC audio file","Protected MPEG-4 video file","Purchased AAC audio file"}'::text,
	'correlation', '0'::real
);


--
-- Statistics for Name: playlist; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist',
	'relpages', '1'::integer,
	'reltuples', '6'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist',
	'attname', 'name',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '10'::integer,
	'n_distinct', '-0.8333333'::real,
	'most_common_vals', '{Music}'::text,
	'most_common_freqs', '{0.33333334}'::real[],
	'histogram_bounds', '{"90s Alternative","Classic Rock","Heavy Metal","TV Shows"}'::text,
	'correlation', '-0.4857143'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist',
	'attname', 'playlist_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{1,3,5,8,10,12}'::text,
	'correlation', '1'::real
);


--
-- Statistics for Name: playlist_track; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_track',
	'relpages', '1'::integer,
	'reltuples', '65'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_track',
	'attname', 'playlist_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '5'::real,
	'most_common_vals', '{5,10,1,12,8}'::text,
	'most_common_freqs', '{0.24615385,0.24615385,0.21538462,0.21538462,0.07692308}'::real[],
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_track',
	'attname', 'track_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-0.7692308'::real,
	'most_common_vals', '{1,302,500,852,3147,20,401,700,800,2428}'::text,
	'most_common_freqs', '{0.046153847,0.046153847,0.046153847,0.046153847,0.046153847,0.03076923,0.03076923,0.03076923,0.03076923,0.03076923}'::real[],
	'histogram_bounds', '{6,15,60,62,300,301,303,310,350,351,352,353,360,361,362,363,400,501,701,702,703,850,851,860,861,950,951,960,2429,2430,2431,2432,2440,2441,2442,3148,3149,3160,3161,3162}'::text,
	'correlation', '0.40104896'::real
);


--
-- Statistics for Name: track; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'relpages', '1'::integer,
	'reltuples', '74'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'album_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-0.2837838'::real,
	'most_common_vals', '{85,193,1,30,35,36,70,80,95,194,4,7,10,31,40,50,51,96,229,230,86}'::text,
	'most_common_freqs', '{0.067567565,0.067567565,0.054054055,0.054054055,0.054054055,0.054054055,0.054054055,0.054054055,0.054054055,0.054054055,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.027027028}'::real[],
	'correlation', '0.8831544'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'bytes',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{5331328,6713451,6714880,6781440,6852860,7032162,7038720,7153873,7197546,7267114,7399453,7452672,7481209,7527387,7547493,7636561,7794560,8192000,8271224,8312734,8353408,8489810,8545280,8595790,8627573,8710048,8734080,8770432,8847549,8896499,8899834,9086866,9228112,9235520,9324870,9577677,9586590,9587588,9611253,9680326,9766211,9832554,9884189,10086400,10152014,10159725,10410302,10498560,10570490,10771104,10925484,11116005,11131066,11170334,11207502,11718866,11984851,12021261,12476835,12530055,12629262,12924665,12960294,13310016,13396710,13531863,13726208,13964083,13990035,14985888,15309312,15712194,16823862,26443800}'::text,
	'correlation', '-0.015683081'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'composer',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '26'::integer,
	'n_distinct', '-0.3918919'::real,
	'most_common_vals', '{"Red Hot Chili Peppers",Radiohead,"Angus Young, Malcolm Young, Brian Johnson",Harris,AC/DC,"Angus Young, Malcolm Young, Bon Scott",Audioslave,"Dave Grohl","Eddie Vedder, Stone Gossard","Foo Fighters","Jimmy Page, Robert Plant","Kurt Cobain","Ritchie Blackmore, Ian Gillan, Roger Glover, Jon Lord, Ian Paice","Roger Waters","James Hetfield, Lars Ulrich, Cliff Burton","James Hetfield, Lars Ulrich, Cliff Burton, Kirk Hammett","James Hetfield, Lars Ulrich, Kirk Hammett","Jimmy Page, Robert Plant, John Paul Jones","Roger Waters, David Gilmour, Rick Wright"}'::text,
	'most_common_freqs', '{0.12162162,0.0945946,0.054054055,0.054054055,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.04054054,0.027027028,0.027027028,0.027027028,0.027027028,0.027027028}'::real[],
	'histogram_bounds', '{Dickinson,"Eddie Vedder, Jeff Ament","James Hetfield, Lars Ulrich","James Hetfield, Lars Ulrich, Cliff Burton, Dave Mustaine","Jimmy Page, Robert Plant, John Paul Jones, John Bonham","Jimmy Page, Robert Plant, John Paul Jones, John Bonham, Memphis Minnie","Kurt Cobain, Krist Novoselic, Dave Grohl","Roger Waters, David Gilmour, Rick Wright, Nick Mason","Roger Waters, Rick Wright","Smith, Dickinson"}'::text,
	'correlation', '0.6957868'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'genre_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '3'::real,
	'most_common_vals', '{1,4,3}'::text,
	'most_common_freqs', '{0.5405405,0.27027026,0.1891892}'::real[],
	'correlation', '0.5498556'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'media_type_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '1'::real,
	'most_common_vals', '{1}'::text,
	'most_common_freqs', '{1}'::real[],
	'correlation', '1'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'milliseconds',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{163424,205662,206080,208192,210834,215196,215680,219219,220917,222380,226013,228384,229181,230619,231307,233926,238837,251000,253416,254693,255973,260101,261867,263301,264359,266893,267728,268693,271804,271960,272796,277890,282764,283010,285753,293407,293631,293720,294034,296672,298290,301296,302826,309040,311072,311285,318981,321671,323069,330133,334743,340745,341163,343373,343719,358982,366654,367377,382305,383853,387186,396199,397104,407823,410506,414824,420493,427946,428669,459180,469176,481826,515386,810554}'::text,
	'correlation', '-0.016512403'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'name',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '15'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{"Aces High",Alive,"Around the World",Battery,Black,"Black Dog","Brain Damage","Breaking The Girl",Breathe,"Bridge Burning",Californication,Cochise,"Come as You Are","Dog Eat Dog","Even Flow",Everlong,"Everything in Its Right Place","Fade to Black","Fight Fire with Fire","For Those About To Rock (We Salute You)","For Whom the Bell Tolls","Funky Monks","Girls Got Rhythm","Give It Away","Go Down","Hallowed Be Thy Name","Highway Star","Highway to Hell",Idioteque,"In Bloom","Inject The Venom",Jeremy,"Karma Police","Kid A","Let''s Get It Up","Let There Be Rock","Like a Stone",Lithium,Lucky,"Master of Puppets",Money,"Monkey Wrench","My Hero","No Quarter","No Surprises",Otherside,"Paranoid Android",Powerslave,"Put The Finger On You","Ride the Lightning","Rock and Roll",Rope,"Run to the Hills","Scar Tissue","Shine On You Crazy Diamond (Parts I-V)","Show Me How to Live","Smells Like Teen Spirit","Smoke on the Water","Space Truckin''","Stairway to Heaven","Suck My Kiss","The Number of the Beast","The Rain Song","The Song Remains the Same","The Thing That Should Not Be",Time,"Touch Too Much","Two Minutes to Midnight","Under the Bridge","Us and Them",Walk,"Welcome Home (Sanitarium)","When the Levee Breaks","Wish You Were Here"}'::text,
	'correlation', '-0.045982968'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'track_id',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '4'::integer,
	'n_distinct', '-1'::real,
	'histogram_bounds', '{1,6,7,8,15,16,17,20,21,22,60,61,62,300,301,302,303,310,311,312,350,351,352,353,360,361,362,363,400,401,402,500,501,502,510,511,512,700,701,702,703,800,801,802,803,850,851,852,853,854,860,861,950,951,952,953,960,961,962,2428,2429,2430,2431,2432,2440,2441,2442,2443,3147,3148,3149,3160,3161,3162}'::text,
	'correlation', '0.8831544'::real
);
SELECT * FROM pg_catalog.pg_restore_attribute_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track',
	'attname', 'unit_price',
	'inherited', 'f'::boolean,
	'null_frac', '0'::real,
	'avg_width', '5'::integer,
	'n_distinct', '1'::real,
	'most_common_vals', '{0.99}'::text,
	'most_common_freqs', '{1}'::real[],
	'correlation', '1'::real
);


--
-- Statistics for Name: album_artist_id_idx; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album_artist_id_idx',
	'relpages', '2'::integer,
	'reltuples', '23'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: album_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'album_pkey',
	'relpages', '2'::integer,
	'reltuples', '23'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: artist_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'artist_pkey',
	'relpages', '2'::integer,
	'reltuples', '20'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: genre_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'genre_pkey',
	'relpages', '2'::integer,
	'reltuples', '10'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: media_type_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'media_type_pkey',
	'relpages', '2'::integer,
	'reltuples', '5'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: playlist_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_pkey',
	'relpages', '2'::integer,
	'reltuples', '6'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: playlist_track_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_track_pkey',
	'relpages', '2'::integer,
	'reltuples', '65'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: playlist_track_playlist_id_idx; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_track_playlist_id_idx',
	'relpages', '2'::integer,
	'reltuples', '65'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: playlist_track_track_id_idx; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'playlist_track_track_id_idx',
	'relpages', '2'::integer,
	'reltuples', '65'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: track_album_id_idx; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track_album_id_idx',
	'relpages', '2'::integer,
	'reltuples', '74'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: track_genre_id_idx; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track_genre_id_idx',
	'relpages', '2'::integer,
	'reltuples', '74'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- Statistics for Name: track_pkey; Type: STATISTICS DATA; Schema: public; Owner: -
--

SELECT * FROM pg_catalog.pg_restore_relation_stats(
	'version', '180003'::integer,
	'schemaname', 'public',
	'relname', 'track_pkey',
	'relpages', '2'::integer,
	'reltuples', '74'::real,
	'relallvisible', '0'::integer,
	'relallfrozen', '0'::integer
);


--
-- PostgreSQL database dump complete
--

\unrestrict OAubpvAxxekVROwIqsQpMT8FI89OvgJRJiI01BBLeC8hikpUN7TuV3dVFAgdEOp

