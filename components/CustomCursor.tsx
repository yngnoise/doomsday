"use client";

import { useEffect, useRef, useState } from "react";
import {
  motion,
  useMotionValue,
  useSpring,
} from "framer-motion";

const FINE_POINTER_QUERY = "(hover: hover) and (pointer: fine)";
const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

export default function CustomCursor() {
  const [enabled, setEnabled] = useState(false);
  const cursorX = useMotionValue(-100);
  const cursorY = useMotionValue(-100);
  const opacity = useMotionValue(0);
  const scale = useMotionValue(1);
  const ringX = useSpring(cursorX, { stiffness: 700, damping: 45, mass: 0.15 });
  const ringY = useSpring(cursorY, { stiffness: 700, damping: 45, mass: 0.15 });
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
      cursorX.set(nextPositionRef.current.x);
      cursorY.set(nextPositionRef.current.y);
      opacity.set(1);
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
      scale.set(0.55);
      if (clickTimerRef.current) clearTimeout(clickTimerRef.current);
      clickTimerRef.current = setTimeout(() => scale.set(1), 120);
    };
    const onPointerLeave = () => opacity.set(0);

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
  }, [cursorX, cursorY, enabled, opacity, scale]);

  if (!enabled) return null;

  return (
    <>
      <motion.div
        data-testid="custom-cursor-ring"
        aria-hidden
        className="fixed top-0 left-0 pointer-events-none z-[9998]"
        style={{
          x: ringX,
          y: ringY,
          opacity,
          translateX: "-50%",
          translateY: "-50%",
          willChange: "transform",
        }}
      >
        <div className="w-7 h-7 rounded-full border border-white/20" />
      </motion.div>
      <motion.div
        data-testid="custom-cursor-crosshair"
        aria-hidden
        className="fixed top-0 left-0 pointer-events-none z-[9999]"
        style={{
          x: cursorX,
          y: cursorY,
          opacity,
          translateX: "-50%",
          translateY: "-50%",
          willChange: "transform",
        }}
      >
        <motion.div className="relative w-[18px] h-[18px]" style={{ scale }}>
          <div className="absolute top-1/2 left-0 right-0 h-px bg-white -translate-y-1/2" />
          <div className="absolute left-1/2 top-0 bottom-0 w-px bg-white -translate-x-1/2" />
          <div className="absolute top-1/2 left-1/2 w-[3px] h-[3px] bg-red-500 rounded-full -translate-x-1/2 -translate-y-1/2" />
        </motion.div>
      </motion.div>
    </>
  );
}
