import type { Metric } from "web-vitals";
import { onCLS, onFCP, onINP, onLCP, onTTFB } from "web-vitals";

/**
 * Logs a Web Vitals metric to the console.
 */
function sendMetric(metric: Metric) {
  console.log(`[Web Vitals] ${metric.name}: ${metric.value.toFixed(2)}`, {
    id: metric.id,
    name: metric.name,
    value: metric.value,
    rating: metric.rating,
    delta: metric.delta,
    navigationType: metric.navigationType,
  });
}

/**
 * Registers all Core Web Vitals metric observers.
 * Call once on client-side mount.
 */
export function reportWebVitals() {
  onCLS(sendMetric);
  onLCP(sendMetric);
  onFCP(sendMetric);
  onTTFB(sendMetric);
  onINP(sendMetric);
}
