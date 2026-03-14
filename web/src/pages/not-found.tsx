import { useNavigate } from "react-router"
import { Button } from "@/components/ui/button"

export default function NotFoundPage() {
  const navigate = useNavigate()

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4">
      <h1 className="text-6xl font-bold text-muted-foreground">404</h1>
      <p className="text-lg text-muted-foreground">页面未找到</p>
      <Button onClick={() => navigate("/")}>返回首页</Button>
    </div>
  )
}
