"use client";

import { useEffect, useRef, useState } from "react";

const FINE_POINTER_QUERY = "(hover: hover) and (pointer: fine)";
const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

export default function CustomCursor() {
  const [enabled, setEnabled] = useState(false);
  const ringRef = useRef<HTMLDivElement>(null);
  const crosshairRef = useRef<HTMLDivElement>(null);
  const crosshairInnerRef = useRef<HTMLDivElement>(null);
  const frameRef = useRef<number | null>(null);
  const clickTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const nextPositionRef = useRef({ x: -100, y: -100 });
  useEffect(() => {
    const finePointer = window.matchMedia(FINE_POINTER_QUERY);
    const reducedMotion = window.matchMedia(REDUCED_MOTION_QUERY);
    const update = () => setEnabled(finePointer.matches && !reducedMotion.matches);

    update();
    finePointer.addEventListener("change", update);
    reducedMotion.addEventListener("change", update);
    return () => {
      finePointer.removeEventListener("change", update);
      reducedMotion.removeEventListener("change", update);
    };
  }, []);

  useEffect(() => {
    document.documentElement.classList.toggle("custom-cursor-active", enabled);
    return () => document.documentElement.classList.remove("custom-cursor-active");
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;

    const flushPosition = () => {
      const { x, y } = nextPositionRef.current;
      const transform = `translate3d(${x}px, ${y}px, 0) translate(-50%, -50%)`;
      if (crosshairRef.current) {
        crosshairRef.current.style.transform = transform;
        crosshairRef.current.style.opacity = "1";
      }
      if (ringRef.current) {
        ringRef.current.style.transform = transform;
        ringRef.current.style.opacity = "1";
      }
      frameRef.current = null;
    };
    const onPointerMove = (event: PointerEvent) => {
      if (event.pointerType !== "mouse") return;
      nextPositionRef.current = { x: event.clientX, y: event.clientY };
      if (frameRef.current === null) {
        frameRef.current = window.requestAnimationFrame(flushPosition);
      }
    };
    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType !== "mouse") return;
      if (crosshairInnerRef.current) crosshairInnerRef.current.style.transform = "scale(0.55)";
      if (clickTimerRef.current) clearTimeout(clickTimerRef.current);
      clickTimerRef.current = setTimeout(() => {
        if (crosshairInnerRef.current) crosshairInnerRef.current.style.transform = "scale(1)";
      }, 120);
    };
    const onPointerLeave = () => {
      if (ringRef.current) ringRef.current.style.opacity = "0";
      if (crosshairRef.current) crosshairRef.current.style.opacity = "0";
    };

    window.addEventListener("pointermove", onPointerMove, { passive: true });
    window.addEventListener("pointerdown", onPointerDown, { passive: true });
    document.documentElement.addEventListener("mouseleave", onPointerLeave);

    return () => {
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerdown", onPointerDown);
      document.documentElement.removeEventListener("mouseleave", onPointerLeave);
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      if (clickTimerRef.current) clearTimeout(clickTimerRef.current);
    };
  }, [enabled]);

  if (!enabled) return null;

  return (
    <>
      <div
        ref={ringRef}
        data-testid="custom-cursor-ring"
        aria-hidden
        className="fixed top-0 left-0 pointer-events-none z-[9998] opacity-0 transition-transform duration-75 ease-out will-change-transform"
      >
        <div className="w-7 h-7 rounded-full border border-white/20" />
      </div>
      <div
        ref={crosshairRef}
        data-testid="custom-cursor-crosshair"
        aria-hidden
        className="fixed top-0 left-0 pointer-events-none z-[9999] opacity-0 will-change-transform"
      >
        <div ref={crosshairInnerRef} className="relative w-[18px] h-[18px] transition-transform duration-100">
          <div className="absolute top-1/2 left-0 right-0 h-px bg-white -translate-y-1/2" />
          <div className="absolute left-1/2 top-0 bottom-0 w-px bg-white -translate-x-1/2" />
          <div className="absolute top-1/2 left-1/2 w-[3px] h-[3px] bg-red-500 rounded-full -translate-x-1/2 -translate-y-1/2" />
        </div>
      </div>
    </>
  );
}
