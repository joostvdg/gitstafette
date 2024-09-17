resource "aws_route53_zone" "gistafette" {
  name = "gitstafette.joostvdg.net"

  tags_all = {
    Environment = "gitstafette"
  }
}

resource "aws_route53_zone" "cmg" {
  name = "cmg.joostvdg.net"

  tags_all = {
    Environment = "cmg"
  }
}

resource "aws_route53_record" "events" {
  zone_id = aws_route53_zone.gistafette.zone_id
  name    = "events.gitstafette.joostvdg.net"
  type    = "A"
  ttl     = 300
  records = [aws_eip.lb.public_ip]
}

resource "aws_route53_record" "cmg-backend" {
  zone_id = aws_route53_zone.cmg.zone_id
  name    = "map.cmg.joostvdg.net"
  type    = "A"
  ttl     = 300
  records = [aws_eip.lb.public_ip]
}
resource "aws_route53_record" "cmg-ui" {
  zone_id = aws_route53_zone.cmg.zone_id
  name    = "be.cmg.joostvdg.net"
  type    = "A"
  ttl     = 300
  records = [aws_eip.lb.public_ip]
}