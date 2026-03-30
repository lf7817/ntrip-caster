import { useEffect, useRef, useState } from "react"
import { useSearchParams } from "react-router"
import { Map, View } from "ol"
import TileLayer from "ol/layer/Tile"
import VectorLayer from "ol/layer/Vector"
import OSM from "ol/source/OSM"
import VectorSource from "ol/source/Vector"
import Feature from "ol/Feature"
import Point from "ol/geom/Point"
import { fromLonLat } from "ol/proj"
import { Style, Circle, Fill, Stroke, Text } from "ol/style"
import { useMountpoints } from "@/api/hooks"
import type { MountpointInfo } from "@/api/types"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

// 基站标记样式
function createMarkerStyle(name: string, isStale: boolean) {
  return new Style({
    image: new Circle({
      radius: 8,
      fill: new Fill({ color: isStale ? "#999" : "#3b82f6" }),
      stroke: new Stroke({ color: "#fff", width: 2 }),
    }),
    text: new Text({
      text: name,
      offsetY: -15,
      fill: new Fill({ color: isStale ? "#666" : "#000" }),
      stroke: new Stroke({ color: "#fff", width: 2 }),
      font: "12px sans-serif",
    }),
  })
}

export default function MapPage() {
  const { data: mounts, isLoading } = useMountpoints()
  const [searchParams] = useSearchParams()
  const highlightMount = searchParams.get("mount")

  const mapRef = useRef<HTMLDivElement>(null)
  const mapObjRef = useRef<Map | null>(null)
  const [selectedMount, setSelectedMount] = useState<MountpointInfo | null>(null)

  // 初始化地图
  useEffect(() => {
    if (!mapRef.current) return

    const map = new Map({
      target: mapRef.current,
      layers: [
        new TileLayer({
          source: new OSM(),
        }),
      ],
      view: new View({
        center: fromLonLat([116.4, 39.9]), // 北京
        zoom: 5,
      }),
    })

    mapObjRef.current = map

    // 点击事件
    map.on("click", (e) => {
      const features = map.getFeaturesAtPixel(e.pixel)
      if (features.length > 0) {
        const feat = features[0] as Feature
        const mountName = feat.get("mountName")
        const mount = mounts?.find(m => m.name === mountName)
        setSelectedMount(mount || null)
      } else {
        setSelectedMount(null)
      }
    })

    return () => map.setTarget(undefined)
  }, [])

  // 添加标记
  useEffect(() => {
    if (!mapObjRef.current || !mounts) return

    const vectorSource = new VectorSource()
    const now = Date.now() / 1000

    mounts.forEach(mp => {
      if (mp.antenna_lat == null || mp.antenna_lon == null) return

      const feat = new Feature({
        geometry: new Point(fromLonLat([mp.antenna_lon, mp.antenna_lat])),
      })
      feat.set("mountName", mp.name)

      // 判断是否过期（> 1 小时）
      const isStale = mp.antenna_updated_at
        ? (now - new Date(mp.antenna_updated_at).getTime() / 1000) > 3600
        : true

      feat.setStyle(createMarkerStyle(mp.name, isStale))
      vectorSource.addFeature(feat)
    })

    const vectorLayer = new VectorLayer({
      source: vectorSource,
    })

    mapObjRef.current.addLayer(vectorLayer)

    // 高亮指定挂载点
    if (highlightMount) {
      const mp = mounts.find(m => m.name === highlightMount)
      if (mp?.antenna_lat && mp?.antenna_lon) {
        mapObjRef.current.getView().animate({
          center: fromLonLat([mp.antenna_lon, mp.antenna_lat]),
          zoom: 12,
          duration: 500,
        })
        setSelectedMount(mp)
      }
    }

    // 清理：移除旧的 vector layer
    return () => {
      mapObjRef.current?.removeLayer(vectorLayer)
    }
  }, [mounts, highlightMount])

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-32" />
        <Skeleton className="h-[500px] w-full" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">基站地图</h1>
        <Badge variant="outline">
          {mounts?.filter(m => m.antenna_lat != null).length || 0} 个有位置数据
        </Badge>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2">
          <div ref={mapRef} className="h-[500px] rounded-md border" />
        </div>

        <div>
          {selectedMount ? (
            <Card>
              <CardHeader>
                <CardTitle>{selectedMount.name}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                <div className="text-sm text-muted-foreground">
                  {selectedMount.description || "无描述"}
                </div>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div>
                    <span className="text-muted-foreground">纬度：</span>
                    {selectedMount.antenna_lat?.toFixed(6)}
                  </div>
                  <div>
                    <span className="text-muted-foreground">经度：</span>
                    {selectedMount.antenna_lon?.toFixed(6)}
                  </div>
                  <div>
                    <span className="text-muted-foreground">高度：</span>
                    {selectedMount.antenna_height?.toFixed(1)} m
                  </div>
                  <div>
                    <span className="text-muted-foreground">更新：</span>
                    {selectedMount.antenna_updated_at || "未知"}
                  </div>
                </div>
                <Badge variant={selectedMount.source_online ? "default" : "secondary"}>
                  {selectedMount.source_online ? "在线" : "离线"}
                </Badge>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="py-8 text-center text-muted-foreground">
                点击地图标记查看基站详情
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}