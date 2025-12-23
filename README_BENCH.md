# 性能对比

## 1. lexer.go性能优化
测试命令：go test -bench=BenchmarkLexer -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof
```
第一轮：
BenchmarkLexerComplex-14        24878343          2867 ns/op         376 B/op         33 allocs/op

第二轮：
BenchmarkLexerComplex-14        35005912          2117 ns/op        1056 B/op         33 allocs/op

第三轮：
BenchmarkLexerComplex-14        51376759          1407 ns/op        1056 B/op         33 allocs/op

第四轮：
BenchmarkLexerSimple-14             	36590464	       315.0 ns/op	     224 B/op	       7 allocs/op
BenchmarkLexerComplex-14            	 8707606	      1386 ns/op	    1056 B/op	      33 allocs/op
BenchmarkLexerStringEscapes-14      	54042236	       219.0 ns/op	     128 B/op	       3 allocs/op
BenchmarkLexerAllTokens-14          	 7791068	      1515 ns/op	     832 B/op	      25 allocs/op
BenchmarkLexerBatchProcessing/单个标记处理-14                     	 7400288	      1519 ns/op	     832 B/op	      25 allocs/op
BenchmarkLexerBatchProcessing/批量标记处理-14                     	 4238590	      2892 ns/op	   10154 B/op	      33 allocs/op

第五轮：
BenchmarkLexerSimple-14             	42068858	       277.8 ns/op	     224 B/op	       7 allocs/op
BenchmarkLexerComplex-14            	 9167444	      1321 ns/op	    1056 B/op	      33 allocs/op
BenchmarkLexerStringEscapes-14      	53771250	       220.5 ns/op	     128 B/op	       3 allocs/op
BenchmarkLexerAllTokens-14          	 8859736	      1379 ns/op	     832 B/op	      25 allocs/op
BenchmarkLexerBatchProcessing/单个标记处理-14                     	 8427406	      1387 ns/op	     832 B/op	      25 allocs/op
BenchmarkLexerBatchProcessing/批量标记处理-14                     	 4499542	      2723 ns/op	   10154 B/op	      33 allocs/op

第六轮：
BenchmarkLexerSimple-14             	40208362	       279.9 ns/op	     224 B/op	       7 allocs/op
BenchmarkLexerComplex-14            	 9223904	      1339 ns/op	    1056 B/op	      33 allocs/op
BenchmarkLexerStringEscapes-14      	55456996	       217.9 ns/op	     128 B/op	       3 allocs/op
BenchmarkLexerAllTokens-14          	 8792521	      1359 ns/op	     832 B/op	      25 allocs/op
BenchmarkLexerBatchProcessing/单个标记处理-14                     	 8836092	      1373 ns/op	     832 B/op	      25 allocs/op
BenchmarkLexerBatchProcessing/批量标记处理-14                     	 4523031	      2782 ns/op	   10154 B/op	      33 allocs/op

第七轮：通过优化批处理字符，提升性能
BenchmarkLexerSimple-14             	56454957	       202.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerComplex-14            	16104240	       746.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerStringEscapes-14      	64603134	       181.3 ns/op	      48 B/op	       1 allocs/op
BenchmarkLexerAllTokens-14          	10290522	      1160 ns/op	      64 B/op	       2 allocs/op
BenchmarkLexerBatchProcessing/单个标记处理-14                     	10088590	      1161 ns/op	      64 B/op	       2 allocs/o

第八轮：数字变成边解析，边计算
BenchmarkLexerSimple-14             	51313683	       226.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerComplex-14            	14934674	       805.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerStringEscapes-14      	64099182	       185.5 ns/op	      48 B/op	       1 allocs/op
BenchmarkLexerAllTokens-14          	 9388269	      1280 ns/op	      64 B/op	       2 allocs/op
BenchmarkLexerBatchProcessing/单个标记处理-14                     	 9565615	      1259 ns/op	      64 B/op	       2 allocs/op
BenchmarkLexerNumbers/整数数组-14                               	38682402	       312.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/浮点数数组-14                              	53380238	       224.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/科学计数法数组-14                            	66874828	       178.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/混合数字JSON-14                           	53085345	       224.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/大数字数组-14                              	36372564	       338.4 ns/op	       0 B/op	       0 allocs/op

第九轮：优化批量处理字符，提升性能
BenchmarkLexerSimple-14             	50412788	       227.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerComplex-14            	14702334	       819.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerStringEscapes-14      	63784083	       189.4 ns/op	      48 B/op	       1 allocs/op
BenchmarkLexerAllTokens-14          	 9191877	      1300 ns/op	      64 B/op	       2 allocs/op
BenchmarkLexerBatchProcessing/单个标记处理-14                     	 9348117	      1287 ns/op	      64 B/op	       2 allocs/op
BenchmarkLexerNumbers/整数数组-14                               	38522527	       314.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/浮点数数组-14                              	51098874	       230.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/科学计数法数组-14                            	66693390	       179.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/混合数字JSON-14                           	53580975	       224.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkLexerNumbers/大数字数组-14                              	36534277	       330.5 ns/op	       0 B/op	       0 allocs/op
```

## 2. parser.go性能优化
测试命令：go test -bench=BenchmarkParser -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof
```
第一轮：
BenchmarkParserSimple-14              	41530178	       279.9 ns/op	     536 B/op	       9 allocs/op
BenchmarkParserComplex-14             	 5541475	      2175 ns/op	    3018 B/op	      62 allocs/op
BenchmarkParserArray-14               	19620277	       639.4 ns/op	    1128 B/op	      25 allocs/op
BenchmarkParserNestedStructures-14    	10833214	      1080 ns/op	    2802 B/op	      33 allocs/op
BenchmarkParserAllTypes-14            	 4809148	      2524 ns/op	    4908 B/op	      59 allocs/op
BenchmarkParserLargeDataset-14        	   84876	    140789 ns/op	  199811 B/op	    3905 allocs/op
BenchmarkParserDirectVsLexer/通过ParseJSON-14             	41691378	       292.7 ns/op	     536 B/op	       9 allocs/op
BenchmarkParserDirectVsLexer/手动Lexer+Parser-14          	42111043	       288.2 ns/op	     536 B/op	       9 allocs/op

第二轮：
BenchmarkParserSimple-14                27664126           437.8 ns/op      1442 B/op         11 allocs/op
BenchmarkParserComplex-14                4069184          2988 ns/op        7549 B/op         72 allocs/op
BenchmarkParserArray-14                 18862125           638.2 ns/op      1120 B/op         24 allocs/op
BenchmarkParserNestedStructures-14       5932318          2116 ns/op        8232 B/op         44 allocs/op
BenchmarkParserAllTypes-14               3982168          3038 ns/op        8817 B/op         65 allocs/op
BenchmarkParserLargeDataset-14             60127        199740 ns/op      471797 B/op       4505 allocs/op
BenchmarkParserDirectVsLexer/通过ParseJSON-14                 26527299           465.0 ns/op      1442 B/op         11 allocs/op
BenchmarkParserDirectVsLexer/手动Lexer+Parser-14              23275177           459.1 ns/op      1442 B/op         11 allocs/op

第三轮：
BenchmarkParserSimple-14              	36080635	       279.6 ns/op	     536 B/op	       9 allocs/op
BenchmarkParserComplex-14             	 5561377	      2160 ns/op	    3018 B/op	      62 allocs/op
BenchmarkParserArray-14               	19883038	       614.5 ns/op	    1120 B/op	      24 allocs/op
BenchmarkParserNestedStructures-14    	10681078	      1086 ns/op	    2794 B/op	      32 allocs/op
BenchmarkParserAllTypes-14            	 4826517	      2507 ns/op	    4892 B/op	      57 allocs/op
BenchmarkParserLargeDataset-14        	   86628	    139245 ns/op	  199802 B/op	    3904 allocs/op
BenchmarkParserDirectVsLexer/通过ParseJSON-14             	41779582	       289.6 ns/op	     536 B/op	       9 allocs/op
BenchmarkParserDirectVsLexer/手动Lexer+Parser-14          	41185029	       293.5 ns/op	     536 B/op	       9 allocs/op

第四轮：
BenchmarkParserSimple-14              	41273463	       282.0 ns/op	     536 B/op	       9 allocs/op
BenchmarkParserComplex-14             	 5553972	      2195 ns/op	    3018 B/op	      62 allocs/op
BenchmarkParserArray-14               	19403274	       627.5 ns/op	    1120 B/op	      24 allocs/op
BenchmarkParserNestedStructures-14    	11341714	      1084 ns/op	    2794 B/op	      32 allocs/op
BenchmarkParserAllTypes-14            	 4808508	      2563 ns/op	    4940 B/op	      58 allocs/op
BenchmarkParserLargeDataset-14        	   83746	    146739 ns/op	  199807 B/op	    3904 allocs/op
BenchmarkParserDirectVsLexer/通过ParseJSON-14             	41826134	       296.0 ns/op	     536 B/op	       9 allocs/op
BenchmarkParserDirectVsLexer/手动Lexer+Parser-14          	41891000	       290.0 ns/op	     536 B/op	       9 allocs/op
```

## 3. sjson.go性能优化
测试命令：go test -bench=BenchmarkCompareMedium -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof
```
第一轮：
BenchmarkComplexJSON/Original-14         	13598248	      5525 ns/op	    9993 B/op	     178 allocs/op
BenchmarkComplexJSON/Optimized-14        	12703338	      5804 ns/op	   10645 B/op	     179 allocs/op
BenchmarkComplexJSON/Standard-14         	17148706	      4125 ns/op	    5136 B/op	     107 allocs/op

第二轮：
BenchmarkComplexDecode/SjsonDecode-14         	 3696825	      3378 ns/op	    5132 B/op	      89 allocs/op
BenchmarkComplexDecode/StdDecode-14           	 4004218	      3055 ns/op	    2632 B/op	      59 allocs/op

第三轮：
BenchmarkComplexDecode/SjsonDecode-14         	 7894840	      1569 ns/op	    2280 B/op	      40 allocs/op
BenchmarkComplexDecode/StdDecode-14           	 4115764	      3026 ns/op	    2632 B/op	      59 allocs/op

第四轮：
BenchmarkComplexEncode/SjsonEncode-14         	 2484872	      4986 ns/op	    5604 B/op	     153 allocs/op
BenchmarkComplexEncode/StdEncode-14           	 6173758	      2010 ns/op	    1841 B/op	      49 allocs/op

第五轮：优化 decode，直接解析，不经过 parser，代码见 sjson_direct_decode.go
BenchmarkDirectVsOldUnmarshal/OldParser-14         	 2506094	      4897 ns/op	    6838 B/op	     133 allocs/op
BenchmarkDirectVsOldUnmarshal/DirectParser-14      	 3106804	      3633 ns/op	    4341 B/op	     110 allocs/op
BenchmarkDirectVsOldUnmarshal/StdlibParser-14      	 2959168	      4138 ns/op	    3392 B/op	      81 allocs/op

第六轮：优化 encode，直接解析，不经过 parser，代码见 sjson_direct_encode.go
BenchmarkDirectComplexTypes/DirectMarshal-Complex-14         	 4028251	      3139 ns/op	    2139 B/op	      69 allocs/op
BenchmarkDirectComplexTypes/Marshal-Complex-14               	 3811060	      3081 ns/op	    2140 B/op	      69 allocs/op
BenchmarkDirectComplexTypes/StdMarshal-Complex-14            	11073609	      1111 ns/op	     880 B/op	      14 allocs/op

第七轮：
BenchmarkCompareMedium/SjsonEncode-14         	31065309	       356.5 ns/op	     472 B/op	       3 allocs/op
BenchmarkCompareMedium/StdEncode-14           	44274247	       244.7 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterEncode-14      	51726506	       233.4 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonDecode-14         	 2090338	      5885 ns/op	    5924 B/op	     115 allocs/op
BenchmarkCompareMedium/StdDecode-14           	 1456482	      8071 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterDecode-14      	 5981205	      2046 ns/op	     384 B/op	      41 allocs/op

第八轮：通过 []byte 优化 encode，提升性能
BenchmarkCompareMedium/SjsonMarshal-14         	35357608	       339.4 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	49267867	       244.6 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	51075765	       238.1 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 1948396	      5828 ns/op	    5924 B/op	     115 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1469923	      8227 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 5809028	      2028 ns/op	     384 B/op	      41 allocs/op

第九轮：通过AI优化部分代码
BenchmarkCompareMedium/SjsonMarshal-14         	45039204	       260.1 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	49513826	       244.3 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	48162411	       240.1 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 3365167	      3371 ns/op	      56 B/op	       4 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1474742	      8185 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 5992104	      2013 ns/op	     352 B/op	      38 allocs/op
```

## 4. 与其他 JSON 库的性能对比
测试命令：go test -bench=BenchmarkCompareMedium -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof

```
第一轮：
BenchmarkCompareMedium/SjsonEncode-14         	 4837003	      2472 ns/op	    4073 B/op	      64 allocs/op
BenchmarkCompareMedium/StdEncode-14           	47469918	       251.2 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterEncode-14      	50867079	       239.8 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonDecode-14         	 2031243	      5920 ns/op	    5924 B/op	     115 allocs/op
BenchmarkCompareMedium/StdDecode-14           	 1475060	      8225 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterDecode-14      	 5912746	      2051 ns/op	     384 B/op	      41 allocs/op

第二轮：
BenchmarkCompareMedium/SjsonMarshal-14         	35773339	       329.0 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	50904760	       248.4 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	48260007	       242.7 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 2104412	      5856 ns/op	    5924 B/op	     115 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1468572	      8210 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 5912791	      2068 ns/op	     352 B/op	      38 allocs/op

第三轮：
BenchmarkCompareMedium/SjsonMarshal-14         	37409500	       316.3 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	51325837	       241.2 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	52111992	       229.6 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 2724361	      4396 ns/op	     168 B/op	       6 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1499334	      7980 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 6126705	      1995 ns/op	     352 B/op	      38 allocs/op

第四轮：通过优化批处理字符，提升性能
BenchmarkCompareMedium/SjsonMarshal-14         	44268858	       260.8 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	49737135	       241.8 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	51249481	       233.8 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 3782358	      3163 ns/op	     184 B/op	       6 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1499564	      8006 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 6064633	      1993 ns/op	     352 B/op	      38 allocs/op

第五轮：
BenchmarkCompareMedium/SjsonMarshal-14         	45357094	       256.8 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	51099282	       232.7 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	52408927	       231.9 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 3605248	      3333 ns/op	     184 B/op	       6 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1457959	      8262 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 6119193	      1993 ns/op	     352 B/op	      38 allocs/op

第六轮：
BenchmarkCompareMedium/SjsonMarshal-14         	43727647	       266.9 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	49850068	       243.0 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	51074415	       237.3 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 3565106	      3364 ns/op	      56 B/op	       4 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1481877	      8174 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 5954317	      1999 ns/op	     352 B/op	      38 allocs/op

第七轮：
BenchmarkCompareMedium/SjsonMarshal-14         	43213803	       258.1 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/StdMarshal-14           	46319899	       241.5 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/JsoniterMarshal-14      	51207300	       237.5 ns/op	     216 B/op	       2 allocs/op
BenchmarkCompareMedium/SjsonUnmarshal-14       	 3606482	      3338 ns/op	      56 B/op	       4 allocs/op
BenchmarkCompareMedium/StdUnmarshal-14         	 1486693	      8076 ns/op	     504 B/op	      11 allocs/op
BenchmarkCompareMedium/JsoniterUnmarshal-14    	 5898898	      2015 ns/op	     352 B/op	      38 allocs/op
```

## 5. byte_utils.go 和 strconv 对比
测试命令：go test -bench=BenchmarkParseIntComparison -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof
```
第一轮：
BenchmarkParseIntComparison/parseIntFromBytes-14         	984710859	        12.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseIntComparison/strconv.ParseInt-14          	651232766	        19.15 ns/op	       0 B/op	       0 allocs/op

第二轮：
BenchmarkParseIntComparison/parseIntFromBytes-14         	1000000000	        10.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseIntComparison/strconv.ParseInt-14          	669233580	        18.44 ns/op	       0 B/op	       0 allocs/op
```

测试命令：go test -bench=BenchmarkParseFloatComparison -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof
```
第一轮：
BenchmarkParseFloatComparison/parseFloatFromBytes-14         	886025876	        13.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseFloatComparison/strconv.ParseFloat-14          	414038884	        28.85 ns/op	       0 B/op	       0 allocs/op

第二轮：
BenchmarkParseFloatComparison/parseFloatFromBytes-14         	875253253	        13.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseFloatComparison/strconv.ParseFloat-14          	411500371	        29.24 ns/op	       0 B/op	       0 allocs/op
```

## 6. 基础类型测试（Unmarshal）
测试命令：go test -bench=BenchmarkUnmarshalCompareTypes -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof
```
第一轮：
BenchmarkUnmarshalCompareTypes/SjsonSimple-14         	158461738	        76.33 ns/op	     152 B/op	       4 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibSimple-14        	100000000	       106.5 ns/op	     176 B/op	       4 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterSimple-14      	152543209	        79.51 ns/op	      40 B/op	       3 allocs/op
BenchmarkUnmarshalCompareTypes/SjsonSmallObject-14    	41844195	       275.4 ns/op	     528 B/op	       9 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibSmallObject-14   	28938656	       423.0 ns/op	     592 B/op	      14 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterSmallObject-14 	52475264	       229.4 ns/op	     464 B/op	      13 allocs/op
BenchmarkUnmarshalCompareTypes/SjsonArray-14          	17696457	       677.7 ns/op	     576 B/op	      25 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibArray-14         	16503360	       737.2 ns/op	     760 B/op	      19 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterArray-14       	39145146	       311.2 ns/op	     600 B/op	      16 allocs/op
BenchmarkUnmarshalCompareTypes/SjsonNestedObject-14   	 9033030	      1297 ns/op	    1849 B/op	      34 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibNestedObject-14  	 6700564	      1817 ns/op	    1984 B/op	      50 allocs/op

第二轮：
BenchmarkUnmarshalCompareTypes/SjsonSimple-14         	154179160	        77.03 ns/op	     152 B/op	       4 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibSimple-14        	100000000	       106.8 ns/op	     176 B/op	       4 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterSimple-14      	152058442	        79.67 ns/op	      40 B/op	       3 allocs/op
BenchmarkUnmarshalCompareTypes/SjsonSmallObject-14    	44412176	       276.6 ns/op	     528 B/op	       9 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibSmallObject-14   	28995601	       421.0 ns/op	     592 B/op	      14 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterSmallObject-14 	50032903	       232.6 ns/op	     464 B/op	      13 allocs/op
BenchmarkUnmarshalCompareTypes/SjsonArray-14          	15086506	       796.4 ns/op	     896 B/op	      27 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibArray-14         	15628522	       735.1 ns/op	     760 B/op	      19 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterArray-14       	38158299	       321.1 ns/op	     600 B/op	      16 allocs/op
BenchmarkUnmarshalCompareTypes/SjsonNestedObject-14   	 9108824	      1353 ns/op	    1849 B/op	      34 allocs/op
BenchmarkUnmarshalCompareTypes/StdlibNestedObject-14  	 6704002	      1813 ns/op	    1984 B/op	      50 allocs/op
BenchmarkUnmarshalCompareTypes/JsoniterNestedObject-14         	10573021	      1152 ns/op	    1977 B/op	      58 allocs/op
```