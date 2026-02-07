INSERT INTO album (album_id, title, artist_id, created_at) VALUES
  -- AC/DC
  (1,   'For Those About To Rock We Salute You', 1,   '2025-11-01T00:00:00Z'),
  (4,   'Let There Be Rock',                     1,   '2025-11-02T00:00:00Z'),
  (7,   'Highway to Hell',                       1,   '2025-11-03T00:00:00Z'),
  -- Audioslave
  (10,  'Audioslave',                             8,   '2025-11-04T00:00:00Z'),
  -- Led Zeppelin
  (30,  'Led Zeppelin IV',                        22,  '2025-11-05T00:00:00Z'),
  (31,  'Houses of the Holy',                     22,  '2025-11-06T00:00:00Z'),
  (32,  'Physical Graffiti',                      22,  '2025-11-07T00:00:00Z'),
  -- Metallica
  (35,  'Master of Puppets',                      50,  '2025-11-08T00:00:00Z'),
  (36,  'Ride the Lightning',                     50,  '2025-11-09T00:00:00Z'),
  (37,  '...And Justice for All',                 50,  '2025-11-10T00:00:00Z'),
  -- Deep Purple
  (40,  'Machine Head',                           58,  '2025-11-11T00:00:00Z'),
  -- Foo Fighters
  (50,  'The Colour and the Shape',               84,  '2025-11-12T00:00:00Z'),
  (51,  'Wasting Light',                          84,  '2025-11-13T00:00:00Z'),
  -- Iron Maiden
  (229, 'Powerslave',                             90,  '2025-11-14T00:00:00Z'),
  (230, 'The Number of the Beast',                90,  '2025-11-15T00:00:00Z'),
  -- Nirvana
  (70,  'Nevermind',                              110, '2025-11-16T00:00:00Z'),
  -- Pearl Jam
  (80,  'Ten',                                    118, '2025-11-17T00:00:00Z'),
  -- Pink Floyd
  (85,  'The Dark Side of the Moon',              120, '2025-11-18T00:00:00Z'),
  (86,  'Wish You Were Here',                     120, '2025-11-19T00:00:00Z'),
  -- Red Hot Chili Peppers
  (193, 'Blood Sugar Sex Magik',                  127, '2025-11-20T00:00:00Z'),
  (194, 'Californication',                        127, '2025-11-21T00:00:00Z'),
  -- Radiohead
  (95,  'OK Computer',                            152, '2025-11-22T00:00:00Z'),
  (96,  'Kid A',                                  152, '2025-11-23T00:00:00Z');

INSERT INTO track (track_id, name, album_id, media_type_id, genre_id, composer, milliseconds, bytes, unit_price) VALUES
  -- For Those About To Rock We Salute You (album 1)
  (1,    'For Those About To Rock (We Salute You)', 1,   1, 1, 'Angus Young, Malcolm Young, Brian Johnson', 343719, 11170334, 0.99),
  (6,    'Put The Finger On You',                   1,   1, 1, 'Angus Young, Malcolm Young, Brian Johnson', 205662, 6713451,  0.99),
  (7,    'Let''s Get It Up',                        1,   1, 1, 'Angus Young, Malcolm Young, Brian Johnson', 233926, 7636561,  0.99),
  (8,    'Inject The Venom',                        1,   1, 1, 'Angus Young, Malcolm Young, Brian Johnson', 210834, 6852860,  0.99),
  -- Let There Be Rock (album 4)
  (15,   'Go Down',                                 4,   1, 1, 'AC/DC',               271804, 8847549,  0.99),
  (16,   'Dog Eat Dog',                             4,   1, 1, 'AC/DC',               215196, 7032162,  0.99),
  (17,   'Let There Be Rock',                       4,   1, 1, 'AC/DC',               366654, 12021261, 0.99),
  -- Highway to Hell (album 7)
  (20,   'Highway to Hell',                         7,   1, 1, 'Angus Young, Malcolm Young, Bon Scott', 208192, 6781440, 0.99),
  (21,   'Girls Got Rhythm',                        7,   1, 1, 'Angus Young, Malcolm Young, Bon Scott', 206080, 6714880, 0.99),
  (22,   'Touch Too Much',                          7,   1, 1, 'Angus Young, Malcolm Young, Bon Scott', 267728, 8734080, 0.99),
  -- Audioslave (album 10)
  (60,   'Cochise',                                 10,  1, 4, 'Audioslave',           222380, 7267114,  0.99),
  (61,   'Show Me How to Live',                     10,  1, 4, 'Audioslave',           277890, 9086866,  0.99),
  (62,   'Like a Stone',                            10,  1, 4, 'Audioslave',           294034, 9611253,  0.99),
  -- Led Zeppelin IV (album 30)
  (300,  'Black Dog',                               30,  1, 1, 'Jimmy Page, Robert Plant, John Paul Jones', 296672, 9680326, 0.99),
  (301,  'Rock and Roll',                           30,  1, 1, 'Jimmy Page, Robert Plant, John Paul Jones, John Bonham', 220917, 7197546, 0.99),
  (302,  'Stairway to Heaven',                      30,  1, 1, 'Jimmy Page, Robert Plant',                 481826, 15712194, 0.99),
  (303,  'When the Levee Breaks',                   30,  1, 1, 'Jimmy Page, Robert Plant, John Paul Jones, John Bonham, Memphis Minnie', 427946, 13964083, 0.99),
  -- Houses of the Holy (album 31)
  (310,  'The Song Remains the Same',               31,  1, 1, 'Jimmy Page, Robert Plant', 330133, 10771104, 0.99),
  (311,  'The Rain Song',                           31,  1, 1, 'Jimmy Page, Robert Plant', 459180, 14985888, 0.99),
  (312,  'No Quarter',                              31,  1, 1, 'Jimmy Page, Robert Plant, John Paul Jones', 420493, 13726208, 0.99),
  -- Master of Puppets (album 35)
  (350,  'Battery',                                 35,  1, 3, 'James Hetfield, Lars Ulrich',  311072, 10152014, 0.99),
  (351,  'Master of Puppets',                       35,  1, 3, 'James Hetfield, Lars Ulrich, Cliff Burton, Kirk Hammett', 515386, 16823862, 0.99),
  (352,  'The Thing That Should Not Be',            35,  1, 3, 'James Hetfield, Lars Ulrich, Kirk Hammett', 396199, 12924665, 0.99),
  (353,  'Welcome Home (Sanitarium)',               35,  1, 3, 'James Hetfield, Lars Ulrich, Kirk Hammett', 387186, 12629262, 0.99),
  -- Ride the Lightning (album 36)
  (360,  'Fight Fire with Fire',                    36,  1, 3, 'James Hetfield, Lars Ulrich, Cliff Burton', 285753, 9324870,  0.99),
  (361,  'Ride the Lightning',                      36,  1, 3, 'James Hetfield, Lars Ulrich, Cliff Burton, Dave Mustaine', 397104, 12960294, 0.99),
  (362,  'For Whom the Bell Tolls',                 36,  1, 3, 'James Hetfield, Lars Ulrich, Cliff Burton', 311285, 10159725, 0.99),
  (363,  'Fade to Black',                           36,  1, 3, 'James Hetfield, Lars Ulrich, Cliff Burton, Kirk Hammett', 414824, 13531863, 0.99),
  -- Machine Head (album 40)
  (400,  'Highway Star',                            40,  1, 1, 'Ritchie Blackmore, Ian Gillan, Roger Glover, Jon Lord, Ian Paice', 367377, 11984851, 0.99),
  (401,  'Smoke on the Water',                      40,  1, 1, 'Ritchie Blackmore, Ian Gillan, Roger Glover, Jon Lord, Ian Paice', 340745, 11116005, 0.99),
  (402,  'Space Truckin''',                         40,  1, 1, 'Ritchie Blackmore, Ian Gillan, Roger Glover, Jon Lord, Ian Paice', 272796, 8899834,  0.99),
  -- The Colour and the Shape (album 50)
  (500,  'Everlong',                                50,  1, 4, 'Dave Grohl',           293631, 9586590,  0.99),
  (501,  'My Hero',                                 50,  1, 4, 'Dave Grohl',           260101, 8489810,  0.99),
  (502,  'Monkey Wrench',                           50,  1, 4, 'Dave Grohl',           231307, 7547493,  0.99),
  -- Wasting Light (album 51)
  (510,  'Bridge Burning',                          51,  1, 4, 'Foo Fighters',         282764, 9228112,  0.99),
  (511,  'Rope',                                    51,  1, 4, 'Foo Fighters',         266893, 8710048,  0.99),
  (512,  'Walk',                                    51,  1, 4, 'Foo Fighters',         302826, 9884189,  0.99),
  -- Powerslave (album 229)
  (3147, 'Aces High',                               229, 1, 3, 'Harris',              271960, 8896499,  0.99),
  (3148, 'Two Minutes to Midnight',                 229, 1, 3, 'Smith, Dickinson',    358982, 11718866, 0.99),
  (3149, 'Powerslave',                              229, 1, 3, 'Dickinson',           407823, 13310016, 0.99),
  -- The Number of the Beast (album 230)
  (3160, 'Run to the Hills',                        230, 1, 3, 'Harris',              230619, 7527387,  0.99),
  (3161, 'The Number of the Beast',                 230, 1, 3, 'Harris',              293407, 9577677,  0.99),
  (3162, 'Hallowed Be Thy Name',                    230, 1, 3, 'Harris',              428669, 13990035, 0.99),
  -- Nevermind (album 70)
  (700,  'Smells Like Teen Spirit',                 70,  1, 4, 'Kurt Cobain, Krist Novoselic, Dave Grohl', 301296, 9832554,  0.99),
  (701,  'Come as You Are',                         70,  1, 4, 'Kurt Cobain',          219219, 7153873,  0.99),
  (702,  'Lithium',                                 70,  1, 4, 'Kurt Cobain',          253416, 8271224,  0.99),
  (703,  'In Bloom',                                70,  1, 4, 'Kurt Cobain',          254693, 8312734,  0.99),
  -- Ten (album 80)
  (800,  'Alive',                                   80,  1, 1, 'Eddie Vedder, Stone Gossard', 341163, 11131066, 0.99),
  (801,  'Even Flow',                               80,  1, 1, 'Eddie Vedder, Stone Gossard', 293720, 9587588,  0.99),
  (802,  'Jeremy',                                  80,  1, 1, 'Eddie Vedder, Jeff Ament',    318981, 10410302, 0.99),
  (803,  'Black',                                   80,  1, 1, 'Eddie Vedder, Stone Gossard', 343373, 11207502, 0.99),
  -- The Dark Side of the Moon (album 85)
  (850,  'Breathe',                                 85,  1, 1, 'Roger Waters, David Gilmour, Rick Wright', 163424, 5331328, 0.99),
  (851,  'Time',                                    85,  1, 1, 'Roger Waters, David Gilmour, Rick Wright, Nick Mason', 410506, 13396710, 0.99),
  (852,  'Money',                                   85,  1, 1, 'Roger Waters',        382305, 12476835, 0.99),
  (853,  'Us and Them',                             85,  1, 1, 'Roger Waters, Rick Wright', 469176, 15309312, 0.99),
  (854,  'Brain Damage',                            85,  1, 1, 'Roger Waters',        228384, 7452672, 0.99),
  -- Wish You Were Here (album 86)
  (860,  'Shine On You Crazy Diamond (Parts I-V)',  86,  1, 1, 'Roger Waters, David Gilmour, Rick Wright', 810554, 26443800, 0.99),
  (861,  'Wish You Were Here',                      86,  1, 1, 'Roger Waters',        334743, 10925484, 0.99),
  -- Blood Sugar Sex Magik (album 193)
  (2428, 'Breaking The Girl',                       193, 1, 1, 'Red Hot Chili Peppers', 298290, 9766211,  0.99),
  (2429, 'Funky Monks',                             193, 1, 1, 'Red Hot Chili Peppers', 323069, 10570490, 0.99),
  (2430, 'Suck My Kiss',                            193, 1, 1, 'Red Hot Chili Peppers', 226013, 7399453,  0.99),
  (2431, 'Give It Away',                            193, 1, 1, 'Red Hot Chili Peppers', 283010, 9235520,  0.99),
  (2432, 'Under the Bridge',                        193, 1, 1, 'Red Hot Chili Peppers', 264359, 8627573,  0.99),
  -- Californication (album 194)
  (2440, 'Around the World',                        194, 1, 1, 'Red Hot Chili Peppers', 238837, 7794560,  0.99),
  (2441, 'Californication',                         194, 1, 1, 'Red Hot Chili Peppers', 321671, 10498560, 0.99),
  (2442, 'Otherside',                               194, 1, 1, 'Red Hot Chili Peppers', 255973, 8353408,  0.99),
  (2443, 'Scar Tissue',                             194, 1, 1, 'Red Hot Chili Peppers', 215680, 7038720,  0.99),
  -- OK Computer (album 95)
  (950,  'Paranoid Android',                        95,  1, 4, 'Radiohead',            383853, 12530055, 0.99),
  (951,  'Karma Police',                            95,  1, 4, 'Radiohead',            263301, 8595790,  0.99),
  (952,  'No Surprises',                            95,  1, 4, 'Radiohead',            229181, 7481209,  0.99),
  (953,  'Lucky',                                   95,  1, 4, 'Radiohead',            268693, 8770432,  0.99),
  -- Kid A (album 96)
  (960,  'Everything in Its Right Place',           96,  1, 4, 'Radiohead',            251000, 8192000,  0.99),
  (961,  'Kid A',                                   96,  1, 4, 'Radiohead',            261867, 8545280,  0.99),
  (962,  'Idioteque',                               96,  1, 4, 'Radiohead',            309040, 10086400, 0.99);
