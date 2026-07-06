"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";

// ─────────────────────────────────────────────────────────────────────────────
// TYPES
// ─────────────────────────────────────────────────────────────────────────────
type AuthStep = "email" | "code";

interface AuthModalProps {
  open: boolean;
  onSuccess: (token: string, email: string) => void;
  onClose: () => void;
}

// ─────────────────────────────────────────────────────────────────────────────
// 6-DIGIT CODE INPUT
// ─────────────────────────────────────────────────────────────────────────────
function CodeInput({ onComplete, disabled }: {
  onComplete: (code: string) => void;
  disabled: boolean;
}) {
  const [digits, setDigits] = useState<string[]>(Array(6).fill(""));
  const refs = useRef<(HTMLInputElement | null)[]>([]);

  // Auto-focus first box on mount
  useEffect(() => { refs.current[0]?.focus(); }, []);

  // When all 6 digits are filled — submit automatically
  useEffect(() => {
    if (digits.every(d => d !== "")) {
      onComplete(digits.join(""));
    }
  }, [digits, onComplete]);

  const handleKeyDown = (i: number, e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace") {
      e.preventDefault();
      if (digits[i] !== "") {
        // Clear current
        setDigits(prev => { const n = [...prev]; n[i] = ""; return n; });
      } else if (i > 0) {
        // Move back and clear previous
        refs.current[i - 1]?.focus();
        setDigits(prev => { const n = [...prev]; n[i - 1] = ""; return n; });
      }
    }
    if (e.key === "ArrowLeft"  && i > 0) refs.current[i - 1]?.focus();
    if (e.key === "ArrowRight" && i < 5) refs.current[i + 1]?.focus();
  };

  const handleChange = (i: number, val: string) => {
    // Handle paste of full code
    if (val.length === 6 && /^\d{6}$/.test(val)) {
      setDigits(val.split(""));
      refs.current[5]?.focus();
      return;
    }
    const digit = val.replace(/\D/g, "").slice(-1);
    setDigits(prev => { const n = [...prev]; n[i] = digit; return n; });
    if (digit && i < 5) refs.current[i + 1]?.focus();
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    e.preventDefault();
    const text = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, 6);
    if (text.length === 6) {
      setDigits(text.split(""));
      refs.current[5]?.focus();
    }
  };

  return (
    <div className="flex gap-3 justify-center">
      {digits.map((d, i) => (
        <motion.input
          key={i}
          ref={el => { refs.current[i] = el; }}
          type="text"
          inputMode="numeric"
          maxLength={6}
          value={d}
          disabled={disabled}
          onChange={e => handleChange(i, e.target.value)}
          onKeyDown={e => handleKeyDown(i, e)}
          onPaste={handlePaste}
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: i * 0.05, duration: 0.25, ease: [0.16, 1, 0.3, 1] }}
          className={[
            "w-11 h-14 text-center text-2xl font-black text-white bg-black",
            "border focus:outline-none tabular-nums transition-colors duration-100",
            "font-mono disabled:opacity-40",
            d !== ""
              ? "border-white"
              : "border-zinc-700 focus:border-zinc-400",
          ].join(" ")}
          style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}
        />
      ))}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// RESEND TIMER
// ─────────────────────────────────────────────────────────────────────────────
function ResendTimer({ onResend }: { onResend: () => void }) {
  const [seconds, setSeconds] = useState(60);

  useEffect(() => {
    if (seconds <= 0) return;
    const id = setTimeout(() => setSeconds(s => s - 1), 1000);
    return () => clearTimeout(id);
  }, [seconds]);

  if (seconds > 0) {
    return (
      <p className="text-xs font-mono text-zinc-600 tracking-widest text-center">
        Resend in{" "}
        <span className="text-zinc-400 tabular-nums">
          0:{String(seconds).padStart(2, "0")}
        </span>
      </p>
    );
  }

  return (
    <button onClick={() => { onResend(); setSeconds(60); }}
      className="text-xs font-mono tracking-widest uppercase text-zinc-400 hover:text-white transition-colors text-center w-full">
      ↺ Resend Code
    </button>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// AUTH MODAL
// ─────────────────────────────────────────────────────────────────────────────
export default function AuthModal({ open, onSuccess, onClose }: AuthModalProps) {
  const [step,        setStep]        = useState<AuthStep>("email");
  const [email,       setEmail]       = useState("");
  const [maskedEmail, setMaskedEmail] = useState("");
  const [error,       setError]       = useState("");
  const [loading,     setLoading]     = useState(false);
  const emailRef = useRef<HTMLInputElement>(null);

  // Reset on open
  useEffect(() => {
    if (open) {
      setStep("email"); setEmail(""); setError(""); setLoading(false);
      setTimeout(() => emailRef.current?.focus(), 100);
    }
  }, [open]);

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  const requestOTP = useCallback(async (emailOverride?: string) => {
    const target = emailOverride ?? email;
    if (!target || !target.includes("@")) {
      setError("Enter a valid email address");
      return;
    }
    setLoading(true); setError("");
    try {
      const res = await fetch("/api/auth/request-otp", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: target }),
      });
      const data = await res.json();
      if (!res.ok) { setError(data.error ?? "Failed to send code"); setLoading(false); return; }
      setMaskedEmail(data.masked_email);
      setStep("code");
    } catch {
      setError("Network error — try again");
    }
    setLoading(false);
  }, [email]);

  const verifyOTP = useCallback(async (code: string) => {
    setLoading(true); setError("");
    try {
      const res = await fetch("/api/auth/verify-otp", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code }),
      });
      const data = await res.json();
      if (!res.ok) { setError(data.error ?? "Invalid code"); setLoading(false); return; }
      onSuccess(data.token, data.email);
    } catch {
      setError("Network error — try again");
      setLoading(false);
    }
  }, [email, onSuccess]);

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.2 }}
          className="fixed inset-0 z-50 flex items-center justify-center p-6"
          style={{ background: "rgba(0,0,0,0.85)", backdropFilter: "blur(8px)" }}
          onClick={onClose}
        >
          <motion.div
            initial={{ opacity: 0, y: 24, scale: 0.97 }}
            animate={{ opacity: 1, y: 0,  scale: 1    }}
            exit={{    opacity: 0, y: 16, scale: 0.97 }}
            transition={{ duration: 0.28, ease: [0.16, 1, 0.3, 1] }}
            className="w-full max-w-sm border border-zinc-700 bg-black p-8 space-y-6 relative"
            onClick={e => e.stopPropagation()}
            style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}
          >
            {/* Close */}
            <button onClick={onClose}
              className="absolute top-4 right-4 text-zinc-600 hover:text-white text-lg leading-none transition-colors">
              ✕
            </button>

            {/* Header */}
            <div>
              <p className="text-xs font-mono tracking-widest uppercase text-zinc-600 mb-2">
                DOOMSDAY™
              </p>
              <AnimatePresence mode="wait">
                {step === "email" ? (
                  <motion.h2 key="h-email"
                    initial={{ opacity: 0, x: -8 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 8 }}
                    transition={{ duration: 0.2 }}
                    className="text-3xl font-black text-white uppercase leading-none"
                    style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
                    Request Access
                  </motion.h2>
                ) : (
                  <motion.h2 key="h-code"
                    initial={{ opacity: 0, x: -8 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 8 }}
                    transition={{ duration: 0.2 }}
                    className="text-3xl font-black text-white uppercase leading-none"
                    style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
                    Enter Access Code
                  </motion.h2>
                )}
              </AnimatePresence>
            </div>

            {/* Body */}
            <AnimatePresence mode="wait">
              {step === "email" ? (
                <motion.div key="step-email" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
                  className="space-y-4">
                  <p className="text-xs font-mono text-zinc-500 leading-relaxed">
                    Enter your email to receive a one-time access code.
                    No password. No account.
                  </p>
                  <input
                    ref={emailRef}
                    type="email"
                    value={email}
                    onChange={e => { setEmail(e.target.value); setError(""); }}
                    onKeyDown={e => e.key === "Enter" && requestOTP()}
                    placeholder="your@email.com"
                    className="w-full h-12 bg-zinc-950 border border-zinc-700 text-white text-sm font-mono px-4 focus:outline-none focus:border-zinc-400 placeholder:text-zinc-700 tracking-wide"
                  />
                  <button
                    onClick={() => requestOTP()}
                    disabled={loading}
                    className="group relative w-full h-12 overflow-hidden border border-white text-sm font-mono font-bold tracking-widest uppercase text-white transition-colors disabled:opacity-50 disabled:cursor-wait"
                  >
                    <motion.span aria-hidden
                      className="absolute inset-0 bg-white origin-left z-0"
                      initial={{ scaleX: 0 }} whileHover={{ scaleX: 1 }}
                      transition={{ duration: 0.2, ease: [0.16, 1, 0.3, 1] }} />
                    <span className="relative z-10 group-hover:text-black transition-colors duration-150">
                      {loading ? "Sending…" : "Send Code →"}
                    </span>
                  </button>
                </motion.div>
              ) : (
                <motion.div key="step-code" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
                  className="space-y-6">
                  <div className="space-y-1">
                    <p className="text-xs font-mono text-zinc-500">Code sent to</p>
                    <p className="text-sm font-mono text-white">{maskedEmail}</p>
                  </div>

                  <CodeInput onComplete={verifyOTP} disabled={loading} />

                  {loading && (
                    <div className="flex justify-center">
                      <motion.div animate={{ rotate: 360 }} transition={{ duration: 0.8, repeat: Infinity, ease: "linear" }}
                        className="w-5 h-5 border-2 border-zinc-600 border-t-white rounded-full" />
                    </div>
                  )}

                  <ResendTimer onResend={() => requestOTP(email)} />

                  <button onClick={() => { setStep("email"); setError(""); }}
                    className="text-xs font-mono tracking-widest uppercase text-zinc-600 hover:text-zinc-400 transition-colors w-full text-center">
                    ← Change Email
                  </button>
                </motion.div>
              )}
            </AnimatePresence>

            {/* Error */}
            <AnimatePresence>
              {error && (
                <motion.p
                  initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="text-xs font-mono text-red-400 tracking-widest overflow-hidden">
                  ✕ {error}
                </motion.p>
              )}
            </AnimatePresence>

            {/* Corner marks */}
            {(["top-0 left-0 border-t border-l","top-0 right-0 border-t border-r",
               "bottom-0 left-0 border-b border-l","bottom-0 right-0 border-b border-r"] as const).map(cls => (
              <div key={cls} className={`absolute w-4 h-4 ${cls} border-zinc-600`} />
            ))}
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}