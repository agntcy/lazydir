class Lazydir < Formula
  desc "Terminal UI for browsing AGNTCY Directory instances"
  homepage "https://github.com/agntcy/lazydir"
  version "v0.0.0"
  license "Apache-2.0"
  version_scheme 1

  url "https://github.com/agntcy/lazydir/releases/download/#{version}"

  on_macos do
      if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-darwin-arm64"
          sha256 "0000000000000000000000000000000000000000000000000000000000000000"

          def install
              bin.install "lazydir-darwin-arm64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end

      if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-darwin-amd64"
          sha256 "0000000000000000000000000000000000000000000000000000000000000000"

          def install
              bin.install "lazydir-darwin-amd64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end
  end

  on_linux do
      if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-linux-arm64"
          sha256 "0000000000000000000000000000000000000000000000000000000000000000"

          def install
              bin.install "lazydir-linux-arm64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end

      if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
          url "#{url}/lazydir-linux-amd64"
          sha256 "0000000000000000000000000000000000000000000000000000000000000000"

          def install
              bin.install "lazydir-linux-amd64" => "lazydir"
              system "chmod", "+x", bin/"lazydir"
          end
      end
  end
end
