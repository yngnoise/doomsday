"use client";

import Image, { type ImageProps } from "next/image";
import { useEffect, useState } from "react";

import { FALLBACK_PRODUCT_IMAGE } from "@/lib/productImages";

type SafeProductImageProps = Omit<ImageProps, "src"> & {
  src: string;
  fallbackSrc?: string;
};

export default function SafeProductImage({
  src,
  fallbackSrc = FALLBACK_PRODUCT_IMAGE,
  onError,
  unoptimized,
  ...props
}: SafeProductImageProps) {
  const [currentSrc, setCurrentSrc] = useState(src);

  useEffect(() => {
    setCurrentSrc(src);
  }, [src]);

  return (
    <Image
      {...props}
      src={currentSrc}
      unoptimized={unoptimized ?? currentSrc.endsWith(".svg")}
      onError={(event) => {
        onError?.(event);
        if (currentSrc !== fallbackSrc) {
          setCurrentSrc(fallbackSrc);
        }
      }}
    />
  );
}
