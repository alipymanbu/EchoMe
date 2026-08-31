import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
const CopyPlugin = require("copy-webpack-plugin");

const nextConfig: NextConfig = {
  output: "standalone",
  images: {
    // 后端返回的预签名 URL 可能来自任意 S3 兼容 endpoint。
    unoptimized: true,
    remotePatterns: [
      {
        protocol: "https",
        hostname: "api.dicebear.com",
      },
    ],
  },
  async rewrites() {
    return [
      {
        source: "/v1/api/:path*",
        destination: `${process.env.API_BASE_URL}/api/:path*`,
      },
    ];
  },
  webpack: (config) => {
    config.ignoreWarnings = [
      ...(config.ignoreWarnings || []),
      /Critical dependency: require function is used in a way in which dependencies cannot be statically extracted/,
    ];
    config.plugins.push(
      new CopyPlugin({
        patterns: [
          {
            from: "node_modules/onnxruntime-web/dist/*.wasm",
            to: "../public/vad/[name][ext]",
          },
          {
            from: "node_modules/@ricky0123/vad-web/dist/vad.worklet.bundle.min.js",
            to: "../public/vad/[name][ext]",
          },
          {
            from: "node_modules/@ricky0123/vad-web/dist/*.onnx",
            to: "../public/vad/[name][ext]",
          },
          {
            from: "node_modules/onnxruntime-web/dist/*.mjs",
            to: "../public/vad/[name][ext]",
          },
        ],
      }),
    );

    return config;
  },
};

const withNextIntl = createNextIntlPlugin("./configs/i18n.ts");
export default withNextIntl(nextConfig);
