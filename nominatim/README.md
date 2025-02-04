
Nominatim Manual API
https://nominatim.org/release-docs/develop/api/Overview/


docker 文件
https://github.com/mediagis/nominatim-docker/blob/master/4.5/README.md


repo
https://github.com/osm-search/Nominatim




# 測試

第三方API可免費直接呼叫 但禁止商业用途
curl 'https://nominatim.openstreetmap.org/reverse?lat=42.602174&lon=-82.512084&format=json'


local monaco
http://127.0.0.1:6080/reverse?lat=43.73812594384041&lon=7.421303955561255&format=json


local taiwan
http://127.0.0.1:6080/reverse?lat=25.02505236216136&lon=121.54914696085176&format=json






# 安裝 osmium-tool
``` bash
brew install osmium-tool
```



# 完整地球下載：
https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf


aria2c -x 16 -s 16 https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf


從 OSM 官方下載變更檔（每日、每小時或每分鐘更新檔）：
	•	日更新檔：https://planet.openstreetmap.org/replication/day/
	•	時更新檔：https://planet.openstreetmap.org/replication/hour/
	•	分鐘更新檔：https://planet.openstreetmap.org/replication/minute/




# 提取經緯度和地區相關資料：
``` bash
osmium tags-filter planet-241118.osm.pbf n/place -o planet.osm.pbf                            # 275M 無效
osmium tags-filter planet-241118.osm.pbf nwr/place -o planet.osm.pbf                          # 1.29G 部分可用
osmium tags-filter planet-241118.osm.pbf nwr/place=country nwr/place=region -o planet.osm.pbf # 113.9M
osmium tags-filter planet-241118.osm.pbf r/type=boundary r/admin_level=2 -o planet.osm.pbf    # 1.33G
```

