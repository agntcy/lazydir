class Lazydir < Formula
  desc "Terminal UI for browsing AGNTCY Directory instances"
  homepage "https://github.com/agntcy/lazydir"
  version "v0.0.2"
  license "Apache-2.0"
  version_scheme 1

  url "https://github.com/agntcy/lazydir/releases/download/#{version}"

  on_macos do
      if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-darwin-arm64"
          sha256 "0036dd449e3e6e42c6e63abf1d688b8eef3d24119c8ef49b722284eb614e13c4"

          def install
              bin.install "lazydir-darwin-arm64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end

      if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-darwin-amd64"
          sha256 "a839f4ec03550c978af395b593a9d83a3c5f9face336a862067e252353469a8c"

          def install
              bin.install "lazydir-darwin-amd64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end
  end

  on_linux do
      if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-linux-arm64"
          sha256 "d081543953241362f5abee745c48a65d4b31d82b52a6778692071b90824d9749"

          def install
              bin.install "lazydir-linux-arm64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end

      if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-linux-amd64"
          sha256 "e6f00b8262ed77477bf643da686c77ee8833a360cfac479fdcfae23764e71c0c"

          def install
              bin.install "lazydir-linux-amd64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end
  end
end
