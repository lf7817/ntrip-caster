import { useState, useEffect, useMemo, useCallback, useEffectEvent, useRef } from "react"
import { useSearchParams } from "react-router"
import { OSM } from "ol/source"
import VectorSource from "ol/source/Vector"
import Feature from "ol/Feature"
import Point from "ol/geom/Point"
import VectorLayer from "ol/layer/Vector"
import { fromLonLat } from "ol/proj"
import { Style, Circle, Fill, Stroke, Text } from "ol/style"
import MapBrowserEvent from "ol/MapBrowserEvent"
import { unByKey } from "ol/Observable"
import "ol/ol.css"
import "react-openlayers/dist/index.css"
import { Map, View, TileLayer, useMap } from "react-openlayers"
import { useMountpoints } from "@/api/hooks"
import type { MountpointInfo } from "@/api/types"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

// 规则：rendering-hoist-jsx - 静态样式提取到组件外部（但需要参数，保持为函数）

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

// 内部组件：地图标记层
function MarkerLayer({
  mounts,
  onSelect,
}: {
  mounts: MountpointInfo[]
  onSelect: (mp: MountpointInfo) => void
}) {
  const map = useMap()
  const vectorSourceRef = useMemo(() => new VectorSource(), [])
  const vectorLayerRef = useMemo(
    () => new VectorLayer({ source: vectorSourceRef }),
    [vectorSourceRef]
  )

  // 规则：advanced-event-handler-refs - 使用 ref 存储事件处理器依赖的最新值
  const mountsRef = useRef(mounts)
  useEffect(() => {
    mountsRef.current = mounts
  }, [mounts])

  // 规则：advanced-use-latest - 使用 useEffectEvent 获取稳定的回调引用
  const onSelectEvent = useEffectEvent(onSelect)

  // 规则：advanced-init-once - 只初始化一次 layer 和事件监听
  useEffect(() => {
    if (!map) return
    map.addLayer(vectorLayerRef)

    // 点击事件处理 - 使用 mountsRef.current 获取最新值
    const handleClick = (e: MapBrowserEvent<PointerEvent>) => {
      const features = map.getFeaturesAtPixel(e.pixel)
      if (features.length > 0) {
        const feat = features[0] as Feature
        const mountName = feat.get("mountName") as string
        const mp = mountsRef.current.find(m => m.name === mountName)
        if (mp) onSelectEvent(mp)
      }
    }

    // 规则：client-event-listeners - 使用 map.on 返回 key，unByKey 清理
    // @ts-expect-error OpenLayers 类型定义过于严格，"click" 是有效的事件类型
    const eventKey = map.on("click", handleClick)
    return () => {
      map.removeLayer(vectorLayerRef)
      unByKey(eventKey)
    }
  }, [map, vectorLayerRef]) // 规则：advanced-event-handler-refs - 不包含 mounts 和 onSelectEvent

  // 规则：rerender-derived-state - 更新 features 而不重建 layer
  useEffect(() => {
    vectorSourceRef.clear()
    const now = Date.now() / 1000

    mounts.forEach(mp => {
      if (mp.antenna_lat == null || mp.antenna_lon == null) return

      const feat = new Feature({
        geometry: new Point(fromLonLat([mp.antenna_lon, mp.antenna_lat])),
      })
      feat.set("mountName", mp.name)

      const isStale = mp.antenna_updated_at
        ? (now - new Date(mp.antenna_updated_at).getTime() / 1000) > 3600
        : true

      feat.setStyle(createMarkerStyle(mp.name, isStale))
      vectorSourceRef.addFeature(feat)
    })
  }, [mounts, vectorSourceRef])

  return null
}

export default function MapPage() {
  const { data: mountsData, isLoading } = useMountpoints({ limit: 1000 })
  const mounts = mountsData?.data
  const [searchParams] = useSearchParams()
  const highlightMount = searchParams.get("mount")

  // 规则：rerender-move-effect-to-event - 交互状态只在事件处理器中更新
  const [selectedMount, setSelectedMount] = useState<MountpointInfo | null>(null)

  // 规则：rerender-derived-state - 派生数据在渲染时计算
  const mountsWithLocation = useMemo(
    () => mounts?.filter(m => m.antenna_lat != null && m.antenna_lon != null) ?? [],
    [mounts]
  )

  // 规则：rerender-derived-state - 派生显示状态（用户选中优先，URL 参数次之）
  const displayMount = useMemo(() => {
    if (selectedMount) return selectedMount
    if (highlightMount && mounts) {
      return mounts.find(m => m.name === highlightMount) ?? null
    }
    return null
  }, [selectedMount, highlightMount, mounts])

  // 规则：rerender-derived-state - 派生视图中心
  const initialCenter = useMemo(() => {
    if (highlightMount && mounts) {
      const mp = mounts.find(m => m.name === highlightMount)
      if (mp?.antenna_lon && mp?.antenna_lat) {
        return fromLonLat([mp.antenna_lon, mp.antenna_lat])
      }
    }
    return fromLonLat([116.4, 39.9]) // 北京
  }, [highlightMount, mounts])

  const initialZoom = highlightMount ? 15 : 5

  // 规则：rerender-functional-setstate - 简单的 setState，使用 useCallback 保持稳定引用
  const handleSelect = useCallback((mp: MountpointInfo) => {
    setSelectedMount(mp)
  }, [])

  // 规则：rerender-derived-state - 派生有位置数据的数量
  const locationCount = mountsWithLocation.length

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-32" />
        <Skeleton className="h-[500px] w-full" />
      </div>
    )
  }

  return (
    <div className="h-[calc(100vh-4rem)] flex flex-col">
      <div className="flex items-center justify-between px-4 py-2 border-b">
        <h1 className="text-xl font-semibold">基站地图</h1>
        <Badge variant="outline">{locationCount} 个有位置数据</Badge>
      </div>

      <div className="flex-1 relative">
        <div className="absolute inset-0 rounded-md border overflow-hidden">
          <Map style={{ height: "100%", width: "100%" }}>
            <TileLayer source={new OSM()} />
            <MarkerLayer mounts={mountsWithLocation} onSelect={handleSelect} />
            <View center={initialCenter} zoom={initialZoom} />
          </Map>
        </div>

        {displayMount && (
          <Card className="absolute bottom-4 right-4 w-72 shadow-lg z-10">
            <CardHeader className="pb-2">
              <CardTitle className="text-lg">{displayMount.name}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div className="text-muted-foreground truncate">
                {displayMount.description || "无描述"}
              </div>
              <div className="grid grid-cols-2 gap-x-2 gap-y-1">
                <div className="truncate">
                  <span className="text-muted-foreground">纬度：</span>
                  {displayMount.antenna_lat?.toFixed(6)}
                </div>
                <div className="truncate">
                  <span className="text-muted-foreground">经度：</span>
                  {displayMount.antenna_lon?.toFixed(6)}
                </div>
                <div className="truncate">
                  <span className="text-muted-foreground">高度：</span>
                  {displayMount.antenna_height?.toFixed(1)} m
                </div>
                <div className="truncate">
                  <span className="text-muted-foreground">状态：</span>
                  <Badge variant={displayMount.source_online ? "default" : "secondary"} className="ml-1">
                    {displayMount.source_online ? "在线" : "离线"}
                  </Badge>
                </div>
              </div>
              <div className="truncate text-muted-foreground">
                更新：{displayMount.antenna_updated_at || "未知"}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  )
}