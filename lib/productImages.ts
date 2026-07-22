export const FALLBACK_PRODUCT_IMAGE = "/product/fallback.svg";

const PRODUCT_IMAGE_MANIFEST: Record<string, readonly string[]> = {
  "dmsdy-ss25-001": [
    "/product/dmsdy-ss25-001/1.avif",
    "/product/dmsdy-ss25-001/2.avif",
    "/product/dmsdy-ss25-001/3.avif",
  ],
};

export function getProductImages(dropID: string): string[] {
  const images = PRODUCT_IMAGE_MANIFEST[dropID];
  return images?.length ? [...images] : [FALLBACK_PRODUCT_IMAGE];
}

export function getProductPreview(dropID: string): string {
  return getProductImages(dropID)[0];
}
