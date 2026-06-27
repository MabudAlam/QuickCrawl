'use client'

import { useEffect, useRef, createContext, useContext, useState, type ReactNode } from 'react'
import Lenis from '@studio-freight/lenis'

interface SmoothScrollContextValue {
  lenis: Lenis | null
}

const SmoothScrollContext = createContext<SmoothScrollContextValue>({ lenis: null })

export function useSmoothScroll() {
  return useContext(SmoothScrollContext)
}

export function SmoothScrollProvider({ children }: { children: ReactNode }) {
  const lenisRef = useRef<Lenis | null>(null)
  const rafRef = useRef<number>(0)
  const [lenis, setLenis] = useState<Lenis | null>(null)

  useEffect(() => {
    lenisRef.current = new Lenis({
      duration: 1.2,
      easing: (t) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
      orientation: 'vertical',
      gestureOrientation: 'vertical',
      smoothWheel: true,
      wheelMultiplier: 1,
      touchMultiplier: 2,
    })

    setLenis(lenisRef.current)

    function raf(time: number) {
      lenisRef.current?.raf(time)
      rafRef.current = requestAnimationFrame(raf)
    }

    rafRef.current = requestAnimationFrame(raf)

    return () => {
      cancelAnimationFrame(rafRef.current)
      lenisRef.current?.destroy()
    }
  }, [])

  return (
    <SmoothScrollContext.Provider value={{ lenis }}>
      {children}
    </SmoothScrollContext.Provider>
  )
}
